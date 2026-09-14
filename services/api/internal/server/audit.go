package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxCorrectionReasonLength = 1000

type adminCorrectionInput struct {
	CheckinAt  *string `json:"checkinAt"`
	CheckoutAt *string `json:"checkoutAt"`
	Reason     string  `json:"reason"`
}

type attendanceAuditSnapshot struct {
	CheckinAt        time.Time  `json:"checkinAt"`
	CheckoutAt       *time.Time `json:"checkoutAt"`
	WorkedSeconds    *int64     `json:"workedSeconds"`
	HourlyRate       string     `json:"hourlyRate"`
	CalculatedAmount *string    `json:"calculatedAmount"`
	Status           string     `json:"status"`
}

// adminCorrectAttendance changes only the attendance timestamps explicitly supplied
// by an administrator. The original and resulting financial values are persisted in
// the same transaction, so an attendance record is never changed without an audit log.
func (a *API) adminCorrectAttendance(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}

	var in adminCorrectionInput
	if !decodeJSON(w, r, &in) {
		return
	}
	reason := strings.TrimSpace(in.Reason)
	if (in.CheckinAt == nil && in.CheckoutAt == nil) || reason == "" || len(reason) > maxCorrectionReasonLength {
		a.bad(w, "checkinAt or checkoutAt and a reason are required")
		return
	}

	checkinAt, err := parseCorrectionTimestamp(in.CheckinAt)
	if err != nil {
		a.bad(w, "invalid checkinAt")
		return
	}
	checkoutAt, err := parseCorrectionTimestamp(in.CheckoutAt)
	if err != nil {
		a.bad(w, "invalid checkoutAt")
		return
	}
	now := time.Now().UTC()
	if (checkinAt != nil && checkinAt.After(now)) || (checkoutAt != nil && checkoutAt.After(now)) {
		a.bad(w, "attendance timestamps cannot be in the future")
		return
	}

	u := UserFromContext(r.Context())
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())

	before, err := attendanceSnapshotForUpdate(r, tx, r.PathValue("id"), u.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "attendance not found"})
		return
	}
	if err != nil {
		a.internal(w)
		return
	}

	after := before
	if checkinAt != nil {
		after.CheckinAt = *checkinAt
	}
	if checkoutAt != nil {
		after.CheckoutAt = checkoutAt
	}
	if !attendanceTimestampsChanged(before, after) {
		a.bad(w, "attendance timestamps are unchanged")
		return
	}
	if after.CheckoutAt != nil && !after.CheckoutAt.After(after.CheckinAt) {
		a.bad(w, "checkoutAt must be after checkinAt")
		return
	}

	var workedSeconds any
	if after.CheckoutAt == nil {
		after.WorkedSeconds = nil
		after.CalculatedAmount = nil
	} else {
		worked := int64(after.CheckoutAt.Sub(after.CheckinAt) / time.Second)
		after.WorkedSeconds = &worked
		after.Status = "COMPLETED"
		workedSeconds = worked
	}

	beforeJSON, err := json.Marshal(before)
	if err != nil {
		a.internal(w)
		return
	}
	updatedWorked := pgtype.Int8{}
	updatedAmount := pgtype.Text{}
	var updatedStatus string
	err = tx.QueryRow(r.Context(), `UPDATE attendance_sessions
		SET checkin_at=$1,
			checkout_at=$2,
			worked_seconds=$3,
			calculated_amount=CASE WHEN $3 IS NULL THEN NULL ELSE round(($3::numeric / 3600) * hourly_rate, 2) END,
			status=$4,
			updated_at=$5
		WHERE id=$6 AND organization_id=$7
		RETURNING worked_seconds, calculated_amount::text, status`,
		after.CheckinAt, after.CheckoutAt, workedSeconds, after.Status, now, r.PathValue("id"), u.OrganizationID,
	).Scan(&updatedWorked, &updatedAmount, &updatedStatus)
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	after.WorkedSeconds = nullableInt64(updatedWorked)
	after.CalculatedAmount = nullableString(updatedAmount)
	after.Status = updatedStatus
	afterJSON, err := json.Marshal(after)
	if err != nil {
		a.internal(w)
		return
	}

	var auditLogID string
	err = tx.QueryRow(r.Context(), `INSERT INTO audit_logs
		(organization_id, actor_user_id, entity_type, entity_id, action, before_data, after_data, reason)
		VALUES ($1, $2, 'attendance_session', $3, 'ADMIN_CORRECTION', $4::jsonb, $5::jsonb, $6)
		RETURNING id`,
		u.OrganizationID, u.ID, r.PathValue("id"), string(beforeJSON), string(afterJSON), reason,
	).Scan(&auditLogID)
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":               r.PathValue("id"),
		"checkinAt":        after.CheckinAt,
		"checkoutAt":       after.CheckoutAt,
		"workedSeconds":    after.WorkedSeconds,
		"calculatedAmount": after.CalculatedAmount,
		"status":           after.Status,
		"auditLogId":       auditLogID,
	})
}

func parseCorrectionTimestamp(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*value))
	if err != nil {
		return nil, err
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func attendanceSnapshotForUpdate(r *http.Request, tx pgx.Tx, attendanceID, organizationID string) (attendanceAuditSnapshot, error) {
	var snapshot attendanceAuditSnapshot
	var checkoutAt pgtype.Timestamptz
	var workedSeconds pgtype.Int8
	var calculatedAmount pgtype.Text
	err := tx.QueryRow(r.Context(), `SELECT checkin_at, checkout_at, worked_seconds, hourly_rate::text, calculated_amount::text, status
		FROM attendance_sessions
		WHERE id=$1 AND organization_id=$2
		FOR UPDATE`, attendanceID, organizationID,
	).Scan(&snapshot.CheckinAt, &checkoutAt, &workedSeconds, &snapshot.HourlyRate, &calculatedAmount, &snapshot.Status)
	if err != nil {
		return attendanceAuditSnapshot{}, err
	}
	snapshot.CheckoutAt = nullableTimestamp(checkoutAt)
	snapshot.WorkedSeconds = nullableInt64(workedSeconds)
	snapshot.CalculatedAmount = nullableString(calculatedAmount)
	return snapshot, nil
}

func attendanceTimestampsChanged(before, after attendanceAuditSnapshot) bool {
	if !before.CheckinAt.Equal(after.CheckinAt) {
		return true
	}
	if before.CheckoutAt == nil || after.CheckoutAt == nil {
		return before.CheckoutAt != after.CheckoutAt
	}
	return !before.CheckoutAt.Equal(*after.CheckoutAt)
}

func nullableTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := value.Time.UTC()
	return &parsed
}

func nullableInt64(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	parsed := value.Int64
	return &parsed
}

func nullableString(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	parsed := value.String
	return &parsed
}
