package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/filipesvieira/workpin/services/api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userContextKey struct{}

type API struct {
	logger       *slog.Logger
	auth         *auth.Service
	cookieSecure bool
	pool         *pgxpool.Pool
}

// New creates API routes. Authenticated handlers receive organization_id only from the verified session.
func New(logger *slog.Logger, authService *auth.Service, pool *pgxpool.Pool, cookieSecure bool) http.Handler {
	api := &API{logger: logger, auth: authService, pool: pool, cookieSecure: cookieSecure}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("POST /auth/request-otp", api.requestOTP)
	mux.HandleFunc("POST /auth/verify-otp", api.verifyOTP)
	mux.HandleFunc("POST /auth/refresh", api.refresh)
	mux.HandleFunc("POST /auth/logout", api.logout)
	mux.Handle("GET /me", api.requireAuth(http.HandlerFunc(api.me)))
	mux.Handle("GET /workers", api.requireAdmin(http.HandlerFunc(api.listWorkers)))
	mux.Handle("POST /workers", api.requireAdmin(http.HandlerFunc(api.createWorker)))
	mux.Handle("PATCH /workers/{id}", api.requireAdmin(http.HandlerFunc(api.updateWorker)))
	mux.Handle("GET /customers", api.requireAdmin(http.HandlerFunc(api.listCustomers)))
	mux.Handle("POST /customers", api.requireAdmin(http.HandlerFunc(api.createCustomer)))
	mux.Handle("PATCH /customers/{id}", api.requireAdmin(http.HandlerFunc(api.updateCustomer)))
	mux.Handle("GET /locations", api.requireAdmin(http.HandlerFunc(api.listLocations)))
	mux.Handle("POST /locations", api.requireAdmin(http.HandlerFunc(api.createLocation)))
	mux.Handle("PATCH /locations/{id}", api.requireAdmin(http.HandlerFunc(api.updateLocation)))
	mux.Handle("GET /jobs", api.requireAdmin(http.HandlerFunc(api.listJobs)))
	mux.Handle("POST /jobs", api.requireAdmin(http.HandlerFunc(api.createJob)))
	mux.Handle("GET /assignments/my/today", api.requireAuth(http.HandlerFunc(api.myToday)))
	return api.withLogging(mux)
}

func (a *API) requireAdmin(next http.Handler) http.Handler {
	return a.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := UserFromContext(r.Context()).Role
		if role != "OWNER" && role != "ADMIN" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "administrator access required"})
			return
		}
		next.ServeHTTP(w, r)
	}))
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "workpin-api"})
}

func (a *API) requestOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := a.auth.RequestOTP(r.Context(), body.Phone); err != nil {
		writeAuthError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) verifyOTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Phone string `json:"phone"`
		Code  string `json:"code"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	user, session, err := a.auth.VerifyOTP(r.Context(), body.Phone, body.Code)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	a.setSessionCookies(w, session)
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.RefreshCookieName)
	if err != nil {
		writeAuthError(w, auth.ErrUnauthenticated)
		return
	}
	user, session, err := a.auth.Refresh(r.Context(), cookie.Value)
	if err != nil {
		a.clearSessionCookies(w)
		writeAuthError(w, err)
		return
	}
	a.setSessionCookies(w, session)
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	cookie, _ := r.Cookie(auth.AccessCookieName)
	if cookie != nil {
		if err := a.auth.Logout(r.Context(), cookie.Value); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "logout failed"})
			return
		}
	}
	a.clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(UserFromContext(r.Context()))})
}

func (a *API) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.auth == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
			return
		}
		cookie, err := r.Cookie(auth.AccessCookieName)
		if err != nil {
			writeAuthError(w, auth.ErrUnauthenticated)
			return
		}
		user, err := a.auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			writeAuthError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
	})
}

// UserFromContext is the only source of organization_id for future tenant-scoped handlers.
func UserFromContext(ctx context.Context) auth.User {
	user, _ := ctx.Value(userContextKey{}).(auth.User)
	return user
}

func (a *API) setSessionCookies(w http.ResponseWriter, session auth.Session) {
	now := time.Now().UTC()
	http.SetCookie(w, &http.Cookie{Name: auth.AccessCookieName, Value: session.AccessToken, Path: "/", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: session.AccessExpiresAt, MaxAge: int(session.AccessExpiresAt.Sub(now).Seconds())})
	http.SetCookie(w, &http.Cookie{Name: auth.RefreshCookieName, Value: session.RefreshToken, Path: "/auth", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, Expires: session.RefreshExpiresAt, MaxAge: int(session.RefreshExpiresAt.Sub(now).Seconds())})
}

func (a *API) clearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: auth.AccessCookieName, Path: "/", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: auth.RefreshCookieName, Path: "/auth", HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

func (a *API) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		a.logger.Info("http request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return false
	}
	return true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidPhone):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrRateLimited):
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
	case errors.Is(err, auth.ErrInvalidOTP), errors.Is(err, auth.ErrUnauthenticated):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication failed"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication service unavailable"})
	}
}

func publicUser(user auth.User) map[string]string {
	return map[string]string{"id": user.ID, "organizationId": user.OrganizationID, "name": user.Name, "phone": user.Phone, "role": user.Role}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
