package server

import (
	"github.com/filipesvieira/workpin/services/api/internal/realtime"
	"github.com/jackc/pgx/v5"
	"net/http"
	"time"
)

type gpsInput struct {
	AssignmentID    string  `json:"assignmentId"`
	Latitude        float64 `json:"latitude"`
	Longitude       float64 `json:"longitude"`
	AccuracyMeters  float64 `json:"accuracyMeters"`
	DeviceTimestamp string  `json:"deviceTimestamp"`
}

func (a *API) checkIn(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in gpsInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validText(in.AssignmentID) || in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 || in.AccuracyMeters < 0 {
		a.bad(w, "invalid location")
		return
	}
	u := UserFromContext(r.Context())
	now := time.Now().UTC()
	tx, e := a.pool.Begin(r.Context())
	if e != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())
	var worker, job string
	var rate string
	var elat, elon float64
	var radius, maxacc int
	e = tx.QueryRow(r.Context(), `SELECT w.id,j.id,COALESCE(ja.hourly_rate_override::text,w.default_hourly_rate::text),l.latitude,l.longitude,l.geofence_radius_meters,o.max_acceptable_gps_accuracy_meters FROM job_assignments ja JOIN workers w ON w.id=ja.worker_id JOIN jobs j ON j.id=ja.job_id JOIN service_locations l ON l.id=j.service_location_id JOIN organizations o ON o.id=ja.organization_id WHERE ja.id=$1 AND w.user_id=$2 AND ja.organization_id=$3 AND ja.status='ASSIGNED'`, in.AssignmentID, u.ID, u.OrganizationID).Scan(&worker, &job, &rate, &elat, &elon, &radius, &maxacc)
	if e == pgx.ErrNoRows {
		writeJSON(w, 404, map[string]string{"error": "assignment not found"})
		return
	}
	if e != nil {
		a.internal(w)
		return
	}
	var distance float64
	e = tx.QueryRow(r.Context(), `SELECT ST_Distance(ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, location) FROM service_locations WHERE latitude=$3 AND longitude=$4 AND organization_id=$5 LIMIT 1`, in.Longitude, in.Latitude, elat, elon, u.OrganizationID).Scan(&distance)
	if e != nil {
		a.internal(w)
		return
	}
	status := "VERIFIED"
	if in.AccuracyMeters > float64(maxacc) {
		status = "LOW_ACCURACY"
	} else if distance > float64(radius) {
		status = "OUTSIDE_GEOFENCE"
	}
	var id string
	e = tx.QueryRow(r.Context(), `INSERT INTO attendance_sessions(organization_id,job_id,assignment_id,worker_id,checkin_at,hourly_rate,status) VALUES($1,$2,$3,$4,$5,$6::numeric,$7) RETURNING id`, u.OrganizationID, job, in.AssignmentID, worker, now, rate, status).Scan(&id)
	if e != nil {
		a.writeDBError(w, e)
		return
	}
	_, e = tx.Exec(r.Context(), `INSERT INTO attendance_events(organization_id,attendance_session_id,worker_id,type,occurred_at,latitude,longitude,accuracy_meters,expected_latitude,expected_longitude,distance_from_location_meters,verification_status,server_timestamp) VALUES($1,$2,$3,'CHECK_IN',$4,$5,$6,$7,$8,$9,$10,$11,$4)`, u.OrganizationID, id, worker, now, in.Latitude, in.Longitude, in.AccuracyMeters, elat, elon, distance, status)
	if e != nil {
		a.internal(w)
		return
	}
	if e = tx.Commit(r.Context()); e != nil {
		a.internal(w)
		return
	}
	a.hub.Publish(u.OrganizationID, realtime.Event{Type: "attendance.checked_in", OrganizationID: u.OrganizationID, OccurredAt: now.Format(time.RFC3339), Payload: map[string]any{"attendanceId": id, "distanceMeters": distance, "verificationStatus": status}})
	writeJSON(w, 201, map[string]any{"id": id, "checkinAt": now, "distanceMeters": distance, "verificationStatus": status})
}
func (a *API) checkOut(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in gpsInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Latitude < -90 || in.Latitude > 90 || in.Longitude < -180 || in.Longitude > 180 || in.AccuracyMeters < 0 {
		a.bad(w, "invalid location")
		return
	}
	u, now := UserFromContext(r.Context()), time.Now().UTC()
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())
	var worker string
	var checkinAt time.Time
	var elat, elon float64
	var radius, maxAccuracy int
	err = tx.QueryRow(r.Context(), `SELECT s.worker_id,s.checkin_at,l.latitude,l.longitude,l.geofence_radius_meters,o.max_acceptable_gps_accuracy_meters FROM attendance_sessions s JOIN workers w ON w.id=s.worker_id JOIN jobs j ON j.id=s.job_id JOIN service_locations l ON l.id=j.service_location_id JOIN organizations o ON o.id=s.organization_id WHERE s.id=$1 AND s.organization_id=$2 AND w.user_id=$3 AND s.status IN ('OPEN','REVIEW_REQUIRED') FOR UPDATE`, r.PathValue("id"), u.OrganizationID, u.ID).Scan(&worker, &checkinAt, &elat, &elon, &radius, &maxAccuracy)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "open attendance not found"})
		return
	}
	if err != nil {
		a.internal(w)
		return
	}
	var distance float64
	err = tx.QueryRow(r.Context(), `SELECT ST_Distance(ST_SetSRID(ST_MakePoint($1,$2),4326)::geography, location) FROM service_locations WHERE latitude=$3 AND longitude=$4 AND organization_id=$5 LIMIT 1`, in.Longitude, in.Latitude, elat, elon, u.OrganizationID).Scan(&distance)
	if err != nil {
		a.internal(w)
		return
	}
	verification := "VERIFIED"
	if in.AccuracyMeters > float64(maxAccuracy) {
		verification = "LOW_ACCURACY"
	} else if distance > float64(radius) {
		verification = "OUTSIDE_GEOFENCE"
	}
	worked := int64(now.Sub(checkinAt).Seconds())
	if worked < 0 {
		a.internal(w)
		return
	}
	var amount string
	err = tx.QueryRow(r.Context(), `UPDATE attendance_sessions SET checkout_at=$1,worked_seconds=$2,calculated_amount=round(($2::numeric/3600)*hourly_rate,2),status=CASE WHEN $3='VERIFIED' THEN 'COMPLETED' ELSE 'REVIEW_REQUIRED' END,updated_at=$1 WHERE id=$4 RETURNING calculated_amount::text`, now, worked, verification, r.PathValue("id")).Scan(&amount)
	if err != nil {
		a.internal(w)
		return
	}
	_, err = tx.Exec(r.Context(), `INSERT INTO attendance_events(organization_id,attendance_session_id,worker_id,type,occurred_at,latitude,longitude,accuracy_meters,expected_latitude,expected_longitude,distance_from_location_meters,verification_status,server_timestamp) VALUES($1,$2,$3,'CHECK_OUT',$4,$5,$6,$7,$8,$9,$10,$11,$4)`, u.OrganizationID, r.PathValue("id"), worker, now, in.Latitude, in.Longitude, in.AccuracyMeters, elat, elon, distance, verification)
	if err != nil {
		a.internal(w)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w)
		return
	}
	a.hub.Publish(u.OrganizationID, realtime.Event{Type: "attendance.checked_out", OrganizationID: u.OrganizationID, OccurredAt: now.Format(time.RFC3339), Payload: map[string]any{"attendanceId": r.PathValue("id"), "workedSeconds": worked, "calculatedAmount": amount, "verificationStatus": verification}})
	writeJSON(w, http.StatusOK, map[string]any{"checkoutAt": now, "workedSeconds": worked, "calculatedAmount": amount, "distanceMeters": distance, "verificationStatus": verification})
}
