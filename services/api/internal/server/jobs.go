package server

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
	"time"
)

type assignmentInput struct {
	WorkerID           string `json:"workerId"`
	HourlyRateOverride string `json:"hourlyRateOverride"`
}
type jobInput struct {
	CustomerID        string            `json:"customerId"`
	ServiceLocationID string            `json:"serviceLocationId"`
	Title             string            `json:"title"`
	Description       string            `json:"description"`
	ScheduledStartAt  string            `json:"scheduledStartAt"`
	ScheduledEndAt    string            `json:"scheduledEndAt"`
	Assignments       []assignmentInput `json:"assignments"`
}

func (a *API) listJobs(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	rows, err := a.pool.Query(r.Context(), `SELECT j.id,j.title,j.status,j.scheduled_start_at,j.scheduled_end_at,c.name,l.name FROM jobs j JOIN customers c ON c.id=j.customer_id JOIN service_locations l ON l.id=j.service_location_id WHERE j.organization_id=$1 ORDER BY j.scheduled_start_at`, UserFromContext(r.Context()).OrganizationID)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, t, s, cn, ln string
		var start, end time.Time
		if err := rows.Scan(&id, &t, &s, &start, &end, &cn, &ln); err != nil {
			a.internal(w)
			return
		}
		items = append(items, map[string]any{"id": id, "title": t, "status": s, "scheduledStartAt": start, "scheduledEndAt": end, "customerName": cn, "locationName": ln})
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": items})
}
func (a *API) createJob(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in jobInput
	if !decodeJSON(w, r, &in) {
		return
	}
	start, e1 := time.Parse(time.RFC3339, in.ScheduledStartAt)
	end, e2 := time.Parse(time.RFC3339, in.ScheduledEndAt)
	if !validText(in.Title) || !validText(in.CustomerID) || !validText(in.ServiceLocationID) || e1 != nil || e2 != nil || !end.After(start) || len(in.Assignments) == 0 {
		a.bad(w, "invalid job fields")
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO jobs (organization_id,customer_id,service_location_id,title,description,scheduled_start_at,scheduled_end_at,status) SELECT $1,$2,$3,$4,NULLIF($5,''),$6,$7,'SCHEDULED' WHERE EXISTS (SELECT 1 FROM customers WHERE id=$2 AND organization_id=$1) AND EXISTS (SELECT 1 FROM service_locations WHERE id=$3 AND customer_id=$2 AND organization_id=$1) RETURNING id`, org, in.CustomerID, in.ServiceLocationID, strings.TrimSpace(in.Title), in.Description, start.UTC(), end.UTC()).Scan(&id)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "customer or location not found"})
		return
	}
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	for _, x := range in.Assignments {
		if !validText(x.WorkerID) || (x.HourlyRateOverride != "" && !validMoney(x.HourlyRateOverride)) {
			a.bad(w, "invalid assignment")
			return
		}
		tag, err := tx.Exec(r.Context(), `INSERT INTO job_assignments (organization_id,job_id,worker_id,hourly_rate_override) SELECT $1,$2,$3,NULLIF($4,'')::numeric WHERE EXISTS (SELECT 1 FROM workers WHERE id=$3 AND organization_id=$1)`, org, id, x.WorkerID, x.HourlyRateOverride)
		if err != nil || tag.RowsAffected() == 0 {
			a.bad(w, "worker not found or duplicated")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}
func (a *API) myToday(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	u := UserFromContext(r.Context())
	rows, err := a.pool.Query(r.Context(), `SELECT j.id,j.title,j.description,j.scheduled_start_at,j.scheduled_end_at,l.id,l.name,l.address_line_1,l.city,l.latitude,l.longitude,l.geofence_radius_meters,COALESCE(ja.hourly_rate_override::text,w.default_hourly_rate::text) FROM workers w JOIN job_assignments ja ON ja.worker_id=w.id AND ja.status='ASSIGNED' JOIN jobs j ON j.id=ja.job_id AND j.status='SCHEDULED' JOIN service_locations l ON l.id=j.service_location_id WHERE w.user_id=$1 AND w.organization_id=$2 AND j.organization_id=$2 AND j.scheduled_start_at < (date_trunc('day',now() AT TIME ZONE 'UTC') + interval '1 day') AND j.scheduled_end_at >= date_trunc('day',now() AT TIME ZONE 'UTC') ORDER BY j.scheduled_start_at`, u.ID, u.OrganizationID)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, t string
		var d *string
		var st, en time.Time
		var lid, ln, a1, city, rate string
		var lat, lon float64
		var radius int
		if err := rows.Scan(&id, &t, &d, &st, &en, &lid, &ln, &a1, &city, &lat, &lon, &radius, &rate); err != nil {
			a.internal(w)
			return
		}
		items = append(items, map[string]any{"id": id, "title": t, "description": d, "scheduledStartAt": st, "scheduledEndAt": en, "hourlyRate": rate, "location": map[string]any{"id": lid, "name": ln, "addressLine1": a1, "city": city, "latitude": lat, "longitude": lon, "geofenceRadiusMeters": radius}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"assignments": items})
}
