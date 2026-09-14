package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	AccessCookieName  = "workpin_access"
	RefreshCookieName = "workpin_refresh"
	maxOTPAttempts    = 5
	maxOTPRequests    = 5
)

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

var (
	ErrInvalidPhone    = errors.New("phone must be in E.164 format")
	ErrInvalidOTP      = errors.New("invalid or expired code")
	ErrRateLimited     = errors.New("too many verification requests")
	ErrUnauthenticated = errors.New("unauthenticated")
)

type SMSProvider interface {
	SendOTP(context.Context, string, string) error
}

type User struct {
	ID, OrganizationID, Name, Phone, Role string
}

type Session struct {
	AccessToken, RefreshToken         string
	AccessExpiresAt, RefreshExpiresAt time.Time
}

type Service struct {
	pool                  *pgxpool.Pool
	sms                   SMSProvider
	now                   func() time.Time
	accessTTL, refreshTTL time.Duration
}

func NewService(pool *pgxpool.Pool, sms SMSProvider, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{pool: pool, sms: sms, now: func() time.Time { return time.Now().UTC() }, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func NormalizePhone(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	if !e164.MatchString(phone) {
		return "", ErrInvalidPhone
	}
	return phone, nil
}

// RequestOTP intentionally returns nil for an unknown or inactive phone to avoid account enumeration.
func (s *Service) RequestOTP(ctx context.Context, phone string) error {
	phone, err := NormalizePhone(phone)
	if err != nil {
		return err
	}
	var userID string
	err = s.pool.QueryRow(ctx, `SELECT id FROM users WHERE phone_e164 = $1 AND active`, phone).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find user: %w", err)
	}
	var requests int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM auth_otps WHERE user_id = $1 AND created_at >= $2`, userID, s.now().Add(-15*time.Minute)).Scan(&requests); err != nil {
		return fmt.Errorf("rate limit: %w", err)
	}
	if requests >= maxOTPRequests {
		return ErrRateLimited
	}
	code, err := newOTP()
	if err != nil {
		return err
	}
	salt, err := randomBytes(16)
	if err != nil {
		return err
	}
	if _, err = s.pool.Exec(ctx, `INSERT INTO auth_otps (user_id, code_hash, code_salt, expires_at) VALUES ($1, $2, $3, $4)`, userID, hashOTP(salt, code), salt, s.now().Add(10*time.Minute)); err != nil {
		return fmt.Errorf("store otp: %w", err)
	}
	if err := s.sms.SendOTP(ctx, phone, code); err != nil {
		return fmt.Errorf("send otp: %w", err)
	}
	return nil
}

func (s *Service) VerifyOTP(ctx context.Context, phone, code string) (User, Session, error) {
	phone, err := NormalizePhone(phone)
	if err != nil {
		return User{}, Session{}, err
	}
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(code) {
		return User{}, Session{}, ErrInvalidOTP
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return User{}, Session{}, err
	}
	defer tx.Rollback(ctx)
	var user User
	err = tx.QueryRow(ctx, `SELECT id, organization_id, name, phone_e164, role FROM users WHERE phone_e164 = $1 AND active`, phone).Scan(&user.ID, &user.OrganizationID, &user.Name, &user.Phone, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrInvalidOTP
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find user: %w", err)
	}
	var otpID string
	var storedHash, salt []byte
	var attempts int
	err = tx.QueryRow(ctx, `SELECT id, code_hash, code_salt, attempts FROM auth_otps WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > $2 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, user.ID, s.now()).Scan(&otpID, &storedHash, &salt, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrInvalidOTP
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find otp: %w", err)
	}
	if attempts >= maxOTPAttempts || subtle.ConstantTimeCompare(storedHash, hashOTP(salt, code)) != 1 {
		_, _ = tx.Exec(ctx, `UPDATE auth_otps SET attempts = attempts + 1 WHERE id = $1`, otpID)
		if err := tx.Commit(ctx); err != nil {
			return User{}, Session{}, err
		}
		return User{}, Session{}, ErrInvalidOTP
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_otps SET consumed_at = $1 WHERE id = $2`, s.now(), otpID); err != nil {
		return User{}, Session{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET last_login_at = $1, updated_at = $1 WHERE id = $2`, s.now(), user.ID); err != nil {
		return User{}, Session{}, err
	}
	session, err := s.newSession(ctx, tx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (User, Session, error) {
	if refreshToken == "" {
		return User{}, Session{}, ErrUnauthenticated
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, Session{}, err
	}
	defer tx.Rollback(ctx)
	var sessionID, userID string
	err = tx.QueryRow(ctx, `SELECT id, user_id FROM refresh_sessions WHERE refresh_token_hash = $1 AND revoked_at IS NULL AND refresh_expires_at > $2 FOR UPDATE`, hashToken(refreshToken), s.now()).Scan(&sessionID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, Session{}, err
	}
	var user User
	err = tx.QueryRow(ctx, `SELECT id, organization_id, name, phone_e164, role FROM users WHERE id = $1 AND active`, userID).Scan(&user.ID, &user.OrganizationID, &user.Name, &user.Phone, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, Session{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = $1, last_used_at = $1 WHERE id = $2`, s.now(), sessionID); err != nil {
		return User{}, Session{}, err
	}
	session, err := s.newSession(ctx, tx, user.ID)
	if err != nil {
		return User{}, Session{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, Session{}, err
	}
	return user, session, nil
}

func (s *Service) Logout(ctx context.Context, accessToken string) error {
	if accessToken == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = COALESCE(revoked_at, $1) WHERE access_token_hash = $2`, s.now(), hashToken(accessToken))
	return err
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (User, error) {
	if accessToken == "" {
		return User{}, ErrUnauthenticated
	}
	var user User
	err := s.pool.QueryRow(ctx, `SELECT u.id, u.organization_id, u.name, u.phone_e164, u.role FROM refresh_sessions s JOIN users u ON u.id = s.user_id WHERE s.access_token_hash = $1 AND s.revoked_at IS NULL AND s.access_expires_at > $2 AND u.active`, hashToken(accessToken), s.now()).Scan(&user.ID, &user.OrganizationID, &user.Name, &user.Phone, &user.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUnauthenticated
	}
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) newSession(ctx context.Context, tx pgx.Tx, userID string) (Session, error) {
	access, err := newToken()
	if err != nil {
		return Session{}, err
	}
	refresh, err := newToken()
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	session := Session{AccessToken: access, RefreshToken: refresh, AccessExpiresAt: now.Add(s.accessTTL), RefreshExpiresAt: now.Add(s.refreshTTL)}
	_, err = tx.Exec(ctx, `INSERT INTO refresh_sessions (user_id, access_token_hash, refresh_token_hash, access_expires_at, refresh_expires_at) VALUES ($1, $2, $3, $4, $5)`, userID, hashToken(access), hashToken(refresh), session.AccessExpiresAt, session.RefreshExpiresAt)
	return session, err
}

func hashToken(value string) []byte { sum := sha256.Sum256([]byte(value)); return sum[:] }
func hashOTP(salt []byte, code string) []byte {
	sum := sha256.Sum256(append(append([]byte{}, salt...), []byte(code)...))
	return sum[:]
}
func randomBytes(size int) ([]byte, error) {
	result := make([]byte, size)
	_, err := rand.Read(result)
	return result, err
}
func newToken() (string, error) {
	value, err := randomBytes(32)
	return base64.RawURLEncoding.EncodeToString(value), err
}
func newOTP() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}
