package server

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/filipesvieira/workpin/services/api/internal/auth"
	"github.com/jackc/pgx/v5"
)

var money = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)

type workerInput struct {
	Name              string `json:"name"`
	Phone             string `json:"phone"`
	EmployeeCode      string `json:"employeeCode"`
	DefaultHourlyRate string `json:"defaultHourlyRate"`
	Active            bool   `json:"active"`
}
type customerInput struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Email  string `json:"email"`
	Notes  string `json:"notes"`
	Active bool   `json:"active"`
}
type locationInput struct {
	CustomerID           string  `json:"customerId"`
	Name                 string  `json:"name"`
	AddressLine1         string  `json:"addressLine1"`
	AddressLine2         string  `json:"addressLine2"`
	City                 string  `json:"city"`
	PostalCode           string  `json:"postalCode"`
	Country              string  `json:"country"`
	Latitude             float64 `json:"latitude"`
	Longitude            float64 `json:"longitude"`
	GeofenceRadiusMeters int     `json:"geofenceRadiusMeters"`
	Notes                string  `json:"notes"`
	Active               bool    `json:"active"`
}

func validMoney(value string) bool { return money.MatchString(value) }
func validText(value string) bool  { return strings.TrimSpace(value) != "" }
func (a *API) unavailable(w http.ResponseWriter) bool {
	if a.pool == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "database unavailable"})
		return true
	}
	return false
}
func (a *API) bad(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
}

func (a *API) listWorkers(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	rows, err := a.pool.Query(r.Context(), `SELECT w.id,u.name,u.phone_e164,COALESCE(w.employee_code,''),w.default_hourly_rate::text,w.active FROM workers w JOIN users u ON u.id=w.user_id WHERE w.organization_id=$1 ORDER BY u.name`, org)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, name, phone, code, rate string
		var active bool
		if err := rows.Scan(&id, &name, &phone, &code, &rate, &active); err != nil {
			a.internal(w)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "phone": phone, "employeeCode": code, "defaultHourlyRate": rate, "active": active})
	}
	writeJSON(w, http.StatusOK, map[string]any{"workers": items})
}
func (a *API) createWorker(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in workerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	phone, err := auth.NormalizePhone(in.Phone)
	if err != nil || !validText(in.Name) || !validMoney(in.DefaultHourlyRate) {
		a.bad(w, "invalid worker fields")
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())
	var userID, workerID string
	err = tx.QueryRow(r.Context(), `INSERT INTO users (organization_id,name,phone_e164,role,active) VALUES ($1,$2,$3,'WORKER',$4) RETURNING id`, org, strings.TrimSpace(in.Name), phone, in.Active).Scan(&userID)
	if err == nil {
		err = tx.QueryRow(r.Context(), `INSERT INTO workers (organization_id,user_id,employee_code,default_hourly_rate,active) VALUES ($1,$2,NULLIF($3,''),$4::numeric,$5) RETURNING id`, org, userID, strings.TrimSpace(in.EmployeeCode), in.DefaultHourlyRate, in.Active).Scan(&workerID)
	}
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.internal(w)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": workerID})
}
func (a *API) updateWorker(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in workerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	phone, err := auth.NormalizePhone(in.Phone)
	if err != nil || !validText(in.Name) || !validMoney(in.DefaultHourlyRate) {
		a.bad(w, "invalid worker fields")
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	tx, err := a.pool.Begin(r.Context())
	if err != nil {
		a.internal(w)
		return
	}
	defer tx.Rollback(r.Context())
	var userID string
	err = tx.QueryRow(r.Context(), `SELECT user_id FROM workers WHERE id=$1 AND organization_id=$2 FOR UPDATE`, r.PathValue("id"), org).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "worker not found"})
		return
	}
	if err != nil {
		a.internal(w)
		return
	}
	now := time.Now().UTC()
	if _, err = tx.Exec(r.Context(), `UPDATE users SET name=$1,phone_e164=$2,active=$3,updated_at=$4 WHERE id=$5 AND organization_id=$6`, strings.TrimSpace(in.Name), phone, in.Active, now, userID, org); err == nil {
		_, err = tx.Exec(r.Context(), `UPDATE workers SET employee_code=NULLIF($1,''),default_hourly_rate=$2::numeric,active=$3,updated_at=$4 WHERE id=$5 AND organization_id=$6`, strings.TrimSpace(in.EmployeeCode), in.DefaultHourlyRate, in.Active, now, r.PathValue("id"), org)
	}
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		a.writeDBError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listCustomers(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	rows, err := a.pool.Query(r.Context(), `SELECT id,name,COALESCE(phone,''),COALESCE(email,''),COALESCE(notes,''),active FROM customers WHERE organization_id=$1 ORDER BY name`, UserFromContext(r.Context()).OrganizationID)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, n, p, e, no string
		var active bool
		rows.Scan(&id, &n, &p, &e, &no, &active)
		items = append(items, map[string]any{"id": id, "name": n, "phone": p, "email": e, "notes": no, "active": active})
	}
	writeJSON(w, http.StatusOK, map[string]any{"customers": items})
}
func (a *API) createCustomer(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in customerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validText(in.Name) {
		a.bad(w, "name is required")
		return
	}
	var id string
	err := a.pool.QueryRow(r.Context(), `INSERT INTO customers (organization_id,name,phone,email,notes,active) VALUES ($1,$2,NULLIF($3,''),NULLIF($4,''),NULLIF($5,''),$6) RETURNING id`, UserFromContext(r.Context()).OrganizationID, strings.TrimSpace(in.Name), in.Phone, in.Email, in.Notes, in.Active).Scan(&id)
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}
func (a *API) updateCustomer(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in customerInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validText(in.Name) {
		a.bad(w, "name is required")
		return
	}
	tag, err := a.pool.Exec(r.Context(), `UPDATE customers SET name=$1,phone=NULLIF($2,''),email=NULLIF($3,''),notes=NULLIF($4,''),active=$5,updated_at=$6 WHERE id=$7 AND organization_id=$8`, strings.TrimSpace(in.Name), in.Phone, in.Email, in.Notes, in.Active, time.Now().UTC(), r.PathValue("id"), UserFromContext(r.Context()).OrganizationID)
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "customer not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listLocations(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	rows, err := a.pool.Query(r.Context(), `SELECT id,customer_id,name,address_line_1,COALESCE(address_line_2,''),city,COALESCE(postal_code,''),country,latitude,longitude,geofence_radius_meters,COALESCE(notes,''),active FROM service_locations WHERE organization_id=$1 ORDER BY name`, UserFromContext(r.Context()).OrganizationID)
	if err != nil {
		a.internal(w)
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, c, n, a1, a2, city, post, country, notes string
		var lat, lon float64
		var radius int
		var active bool
		rows.Scan(&id, &c, &n, &a1, &a2, &city, &post, &country, &lat, &lon, &radius, &notes, &active)
		items = append(items, map[string]any{"id": id, "customerId": c, "name": n, "addressLine1": a1, "addressLine2": a2, "city": city, "postalCode": post, "country": country, "latitude": lat, "longitude": lon, "geofenceRadiusMeters": radius, "notes": notes, "active": active})
	}
	writeJSON(w, http.StatusOK, map[string]any{"locations": items})
}
func validLocation(in locationInput) bool {
	return validText(in.CustomerID) && validText(in.Name) && validText(in.AddressLine1) && validText(in.City) && len(in.Country) == 2 && in.Latitude >= -90 && in.Latitude <= 90 && in.Longitude >= -180 && in.Longitude <= 180 && in.GeofenceRadiusMeters > 0
}
func (a *API) createLocation(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in locationInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validLocation(in) {
		a.bad(w, "invalid location fields")
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	var id string
	err := a.pool.QueryRow(r.Context(), `INSERT INTO service_locations (organization_id,customer_id,name,address_line_1,address_line_2,city,postal_code,country,latitude,longitude,location,geofence_radius_meters,notes,active) SELECT $1,$2,$3,$4,NULLIF($5,''),$6,NULLIF($7,''),upper($8),$9,$10,ST_SetSRID(ST_MakePoint($10,$9),4326)::geography,$11,NULLIF($12,''),$13 WHERE EXISTS (SELECT 1 FROM customers WHERE id=$2 AND organization_id=$1) RETURNING id`, org, in.CustomerID, strings.TrimSpace(in.Name), strings.TrimSpace(in.AddressLine1), in.AddressLine2, strings.TrimSpace(in.City), in.PostalCode, in.Country, in.Latitude, in.Longitude, in.GeofenceRadiusMeters, in.Notes, in.Active).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "customer not found"})
		return
	}
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}
func (a *API) updateLocation(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	var in locationInput
	if !decodeJSON(w, r, &in) {
		return
	}
	if !validLocation(in) {
		a.bad(w, "invalid location fields")
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	tag, err := a.pool.Exec(r.Context(), `UPDATE service_locations l SET customer_id=$1,name=$2,address_line_1=$3,address_line_2=NULLIF($4,''),city=$5,postal_code=NULLIF($6,''),country=upper($7),latitude=$8,longitude=$9,location=ST_SetSRID(ST_MakePoint($9,$8),4326)::geography,geofence_radius_meters=$10,notes=NULLIF($11,''),active=$12,updated_at=$13 WHERE l.id=$14 AND l.organization_id=$15 AND EXISTS (SELECT 1 FROM customers c WHERE c.id=$1 AND c.organization_id=$15)`, in.CustomerID, strings.TrimSpace(in.Name), strings.TrimSpace(in.AddressLine1), in.AddressLine2, strings.TrimSpace(in.City), in.PostalCode, in.Country, in.Latitude, in.Longitude, in.GeofenceRadiusMeters, in.Notes, in.Active, time.Now().UTC(), r.PathValue("id"), org)
	if err != nil {
		a.writeDBError(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "location or customer not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) internal(w http.ResponseWriter) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "service unavailable"})
}
func (a *API) writeDBError(w http.ResponseWriter, err error) {
	a.logger.Error("database write failed", "error", err)
	a.internal(w)
}
