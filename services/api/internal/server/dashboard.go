package server

import (
	"net/http"
	"time"
)

func (a *API) dashboardToday(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	start := time.Now().UTC().Truncate(24 * time.Hour)
	var workers, working, completed, attention int
	var seconds int64
	var amount string
	e := a.pool.QueryRow(r.Context(), `SELECT (SELECT count(*) FROM workers WHERE organization_id=$1 AND active),(SELECT count(*) FROM attendance_sessions WHERE organization_id=$1 AND status='OPEN'),(SELECT count(*) FROM attendance_sessions WHERE organization_id=$1 AND status='COMPLETED' AND checkin_at >= $2),(SELECT count(*) FROM attendance_sessions WHERE organization_id=$1 AND status='REVIEW_REQUIRED'),COALESCE((SELECT sum(worked_seconds) FROM attendance_sessions WHERE organization_id=$1 AND checkin_at >= $2),0),COALESCE((SELECT sum(calculated_amount)::text FROM attendance_sessions WHERE organization_id=$1 AND checkin_at >= $2),'0.00')`, org, start).Scan(&workers, &working, &completed, &attention, &seconds, &amount)
	if e != nil {
		a.internal(w)
		return
	}
	writeJSON(w, 200, map[string]any{"workersToday": workers, "currentlyWorking": working, "completed": completed, "needsAttention": attention, "workedSeconds": seconds, "estimatedPay": amount})
}

func (a *API) dashboardActivity(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	rows, err := a.pool.Query(r.Context(), `SELECT e.type,e.occurred_at,u.name,e.distance_from_location_meters,e.verification_status FROM attendance_events e JOIN users u ON u.id=(SELECT user_id FROM workers WHERE id=e.worker_id) WHERE e.organization_id=$1 ORDER BY e.occurred_at DESC LIMIT 50`, org)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var typ, name, status string
		var at time.Time
		var distance float64
		if err := rows.Scan(&typ, &at, &name, &distance, &status); err != nil {
			a.internal(w)
			return
		}
		items = append(items, map[string]any{"type": typ, "occurredAt": at, "workerName": name, "distanceMeters": distance, "verificationStatus": status})
	}
	writeJSON(w, 200, map[string]any{"activity": items})
}

func (a *API) dashboardMap(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	rows, err := a.pool.Query(r.Context(), `SELECT e.type,e.occurred_at,e.latitude,e.longitude,e.accuracy_meters,e.distance_from_location_meters,e.verification_status,u.name,l.id,l.name,l.latitude,l.longitude,l.geofence_radius_meters FROM attendance_events e JOIN attendance_sessions s ON s.id=e.attendance_session_id JOIN jobs j ON j.id=s.job_id JOIN service_locations l ON l.id=j.service_location_id JOIN workers w ON w.id=e.worker_id JOIN users u ON u.id=w.user_id WHERE e.organization_id=$1 ORDER BY e.occurred_at DESC LIMIT 500`, org)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var typ, status, worker, lid, lname string
		var at time.Time
		var lat, lon, accuracy, distance, llat, llon float64
		var radius int
		if err := rows.Scan(&typ, &at, &lat, &lon, &accuracy, &distance, &status, &worker, &lid, &lname, &llat, &llon, &radius); err != nil {
			a.internal(w)
			return
		}
		items = append(items, map[string]any{"type": typ, "occurredAt": at, "workerName": worker, "latitude": lat, "longitude": lon, "accuracyMeters": accuracy, "distanceMeters": distance, "verificationStatus": status, "location": map[string]any{"id": lid, "name": lname, "latitude": llat, "longitude": llon, "geofenceRadiusMeters": radius}})
	}
	writeJSON(w, 200, map[string]any{"markers": items})
}
