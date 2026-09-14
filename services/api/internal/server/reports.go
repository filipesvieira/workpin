package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var reportUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type timesheetFilters struct {
	from, toExclusive    *time.Time
	fromDate, toDate     string
	workerID, customerID *string
	locationID, status   *string
}

type timesheetRecord struct {
	ID               string     `json:"id"`
	WorkerID         string     `json:"workerId"`
	WorkerName       string     `json:"workerName"`
	EmployeeCode     string     `json:"employeeCode"`
	JobID            string     `json:"jobId"`
	JobTitle         string     `json:"jobTitle"`
	CustomerID       string     `json:"customerId"`
	CustomerName     string     `json:"customerName"`
	LocationID       string     `json:"locationId"`
	LocationName     string     `json:"locationName"`
	CheckinAt        time.Time  `json:"checkinAt"`
	CheckoutAt       *time.Time `json:"checkoutAt"`
	WorkedSeconds    *int64     `json:"workedSeconds"`
	HourlyRate       string     `json:"hourlyRate"`
	CalculatedAmount *string    `json:"calculatedAmount"`
	Status           string     `json:"status"`
	CheckinStatus    *string    `json:"checkinStatus"`
	CheckoutStatus   *string    `json:"checkoutStatus"`
}

type timesheetTotals struct {
	Sessions      int64  `json:"sessions"`
	WorkedSeconds int64  `json:"workedSeconds"`
	EstimatedPay  string `json:"estimatedPay"`
	Currency      string `json:"currency"`
}

type rateSummary struct {
	HourlyRate    string `json:"hourlyRate"`
	Sessions      int64  `json:"sessions"`
	WorkedSeconds int64  `json:"workedSeconds"`
	EstimatedPay  string `json:"estimatedPay"`
}

type workerTimesheetSummary struct {
	WorkerID      string        `json:"workerId"`
	WorkerName    string        `json:"workerName"`
	EmployeeCode  string        `json:"employeeCode"`
	Jobs          int64         `json:"jobs"`
	Sessions      int64         `json:"sessions"`
	WorkedSeconds int64         `json:"workedSeconds"`
	EstimatedPay  string        `json:"estimatedPay"`
	Rates         []rateSummary `json:"rates"`
}

const timesheetJoinSQL = `
FROM attendance_sessions s
JOIN workers w ON w.id = s.worker_id AND w.organization_id = s.organization_id
JOIN users u ON u.id = w.user_id AND u.organization_id = s.organization_id
JOIN jobs j ON j.id = s.job_id AND j.organization_id = s.organization_id
JOIN customers c ON c.id = j.customer_id AND c.organization_id = s.organization_id
JOIN service_locations l ON l.id = j.service_location_id AND l.organization_id = s.organization_id
`

const timesheetFilterSQL = `
WHERE s.organization_id = $1
  AND ($2::timestamptz IS NULL OR s.checkin_at >= $2::timestamptz)
  AND ($3::timestamptz IS NULL OR s.checkin_at < $3::timestamptz)
  AND ($4::uuid IS NULL OR s.worker_id = $4::uuid)
  AND ($5::uuid IS NULL OR j.customer_id = $5::uuid)
  AND ($6::uuid IS NULL OR l.id = $6::uuid)
  AND ($7::text IS NULL OR s.status = $7::text)
`

func (a *API) reportsTimesheets(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	location, currency, err := a.reportSettings(r.Context(), org)
	if err != nil {
		a.internal(w)
		return
	}
	filters, err := parseTimesheetFilters(r, location)
	if err != nil {
		a.bad(w, "invalid report filters")
		return
	}
	records, err := a.loadTimesheets(r.Context(), org, filters)
	if err != nil {
		a.internal(w)
		return
	}
	totals, err := a.loadTimesheetTotals(r.Context(), org, filters, currency)
	if err != nil {
		a.internal(w)
		return
	}
	workers, err := a.loadWorkerTimesheetSummaries(r.Context(), org, filters)
	if err != nil {
		a.internal(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filters":    filters.response(),
		"timesheets": records,
		"totals":     totals,
		"workers":    workers,
	})
}

func (a *API) exportTimesheets(w http.ResponseWriter, r *http.Request) {
	if a.unavailable(w) {
		return
	}
	org := UserFromContext(r.Context()).OrganizationID
	location, _, err := a.reportSettings(r.Context(), org)
	if err != nil {
		a.internal(w)
		return
	}
	filters, err := parseTimesheetFilters(r, location)
	if err != nil {
		a.bad(w, "invalid report filters")
		return
	}
	records, err := a.loadTimesheets(r.Context(), org, filters)
	if err != nil {
		a.internal(w)
		return
	}

	var content bytes.Buffer
	writer := csv.NewWriter(&content)
	if err := writer.Write([]string{"Worker", "Customer", "Location", "Check-in", "Checkout", "Worked time", "Hourly rate", "Amount", "Check-in status", "Checkout status"}); err != nil {
		a.internal(w)
		return
	}
	for _, record := range records {
		checkout := ""
		if record.CheckoutAt != nil {
			checkout = reportTimestamp(*record.CheckoutAt, location)
		}
		amount := ""
		if record.CalculatedAmount != nil {
			amount = *record.CalculatedAmount
		}
		checkinStatus := ""
		if record.CheckinStatus != nil {
			checkinStatus = *record.CheckinStatus
		}
		checkoutStatus := ""
		if record.CheckoutStatus != nil {
			checkoutStatus = *record.CheckoutStatus
		}
		if err := writer.Write([]string{
			csvValue(record.WorkerName),
			csvValue(record.CustomerName),
			csvValue(record.LocationName),
			reportTimestamp(record.CheckinAt, location),
			checkout,
			workedTime(record.WorkedSeconds),
			record.HourlyRate,
			amount,
			checkinStatus,
			checkoutStatus,
		}); err != nil {
			a.internal(w)
			return
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		a.internal(w)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=timesheets.csv")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content.Bytes())
}

func (a *API) reportSettings(ctx context.Context, organizationID string) (*time.Location, string, error) {
	var timezone, currency string
	if err := a.pool.QueryRow(ctx, `SELECT timezone, currency FROM organizations WHERE id = $1`, organizationID).Scan(&timezone, &currency); err != nil {
		return nil, "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, "", err
	}
	return location, currency, nil
}

func parseTimesheetFilters(r *http.Request, location *time.Location) (timesheetFilters, error) {
	query := r.URL.Query()
	filters := timesheetFilters{
		fromDate: strings.TrimSpace(query.Get("from")),
		toDate:   strings.TrimSpace(query.Get("to")),
	}
	var err error
	if filters.workerID, err = reportIDFilter(query.Get("workerId")); err != nil {
		return timesheetFilters{}, err
	}
	if filters.customerID, err = reportIDFilter(query.Get("customerId")); err != nil {
		return timesheetFilters{}, err
	}
	if filters.locationID, err = reportIDFilter(query.Get("locationId")); err != nil {
		return timesheetFilters{}, err
	}
	if value := strings.ToUpper(strings.TrimSpace(query.Get("status"))); value != "" {
		switch value {
		case "OPEN", "COMPLETED", "REVIEW_REQUIRED":
			filters.status = &value
		default:
			return timesheetFilters{}, fmt.Errorf("invalid attendance status")
		}
	}

	var fromDay, toDay *time.Time
	if filters.fromDate != "" {
		value, err := reportDay(filters.fromDate, location)
		if err != nil {
			return timesheetFilters{}, err
		}
		fromDay = &value
		filters.from = &value
	}
	if filters.toDate != "" {
		value, err := reportDay(filters.toDate, location)
		if err != nil {
			return timesheetFilters{}, err
		}
		toDay = &value
		exclusive := value.AddDate(0, 0, 1)
		filters.toExclusive = &exclusive
	}
	if fromDay != nil && toDay != nil && toDay.Before(*fromDay) {
		return timesheetFilters{}, fmt.Errorf("invalid date range")
	}
	return filters, nil
}

func reportDay(value string, location *time.Location) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", value, location)
}

func reportIDFilter(value string) (*string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if !reportUUID.MatchString(value) {
		return nil, fmt.Errorf("invalid identifier")
	}
	return &value, nil
}

func (f timesheetFilters) args(organizationID string) []any {
	return []any{
		organizationID,
		optionalReportTime(f.from),
		optionalReportTime(f.toExclusive),
		optionalReportString(f.workerID),
		optionalReportString(f.customerID),
		optionalReportString(f.locationID),
		optionalReportString(f.status),
	}
}

func optionalReportTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalReportString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func (f timesheetFilters) response() map[string]string {
	return map[string]string{
		"from":       f.fromDate,
		"to":         f.toDate,
		"workerId":   stringValue(f.workerID),
		"customerId": stringValue(f.customerID),
		"locationId": stringValue(f.locationID),
		"status":     stringValue(f.status),
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (a *API) loadTimesheets(ctx context.Context, organizationID string, filters timesheetFilters) ([]timesheetRecord, error) {
	rows, err := a.pool.Query(ctx, `
SELECT s.id, s.worker_id, u.name, COALESCE(w.employee_code, ''),
       j.id, j.title, c.id, c.name, l.id, l.name,
       s.checkin_at, s.checkout_at, s.worked_seconds, s.hourly_rate::text,
       s.calculated_amount::text, s.status,
       checkin.verification_status, checkout.verification_status
`+timesheetJoinSQL+`
LEFT JOIN LATERAL (
    SELECT verification_status
    FROM attendance_events e
    WHERE e.attendance_session_id = s.id AND e.type = 'CHECK_IN'
    ORDER BY e.occurred_at ASC, e.created_at ASC
    LIMIT 1
) checkin ON true
LEFT JOIN LATERAL (
    SELECT verification_status
    FROM attendance_events e
    WHERE e.attendance_session_id = s.id AND e.type = 'CHECK_OUT'
    ORDER BY e.occurred_at DESC, e.created_at DESC
    LIMIT 1
) checkout ON true
`+timesheetFilterSQL+`
ORDER BY s.checkin_at DESC, s.id DESC`, filters.args(organizationID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := []timesheetRecord{}
	for rows.Next() {
		var record timesheetRecord
		if err := rows.Scan(
			&record.ID, &record.WorkerID, &record.WorkerName, &record.EmployeeCode,
			&record.JobID, &record.JobTitle, &record.CustomerID, &record.CustomerName,
			&record.LocationID, &record.LocationName, &record.CheckinAt, &record.CheckoutAt,
			&record.WorkedSeconds, &record.HourlyRate, &record.CalculatedAmount, &record.Status,
			&record.CheckinStatus, &record.CheckoutStatus,
		); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (a *API) loadTimesheetTotals(ctx context.Context, organizationID string, filters timesheetFilters, currency string) (timesheetTotals, error) {
	var totals timesheetTotals
	totals.Currency = currency
	err := a.pool.QueryRow(ctx, `
SELECT count(*), COALESCE(sum(s.worked_seconds), 0), COALESCE(sum(s.calculated_amount)::text, '0.00')
`+timesheetJoinSQL+timesheetFilterSQL, filters.args(organizationID)...).Scan(&totals.Sessions, &totals.WorkedSeconds, &totals.EstimatedPay)
	return totals, err
}

func (a *API) loadWorkerTimesheetSummaries(ctx context.Context, organizationID string, filters timesheetFilters) ([]workerTimesheetSummary, error) {
	rows, err := a.pool.Query(ctx, `
SELECT s.worker_id, u.name, COALESCE(w.employee_code, ''), count(DISTINCT s.job_id), count(*),
       COALESCE(sum(s.worked_seconds), 0), COALESCE(sum(s.calculated_amount)::text, '0.00')
`+timesheetJoinSQL+timesheetFilterSQL+`
GROUP BY s.worker_id, u.name, w.employee_code
ORDER BY u.name, s.worker_id`, filters.args(organizationID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	summaries := []workerTimesheetSummary{}
	indexes := map[string]int{}
	for rows.Next() {
		var summary workerTimesheetSummary
		if err := rows.Scan(&summary.WorkerID, &summary.WorkerName, &summary.EmployeeCode, &summary.Jobs, &summary.Sessions, &summary.WorkedSeconds, &summary.EstimatedPay); err != nil {
			return nil, err
		}
		summary.Rates = []rateSummary{}
		indexes[summary.WorkerID] = len(summaries)
		summaries = append(summaries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rateRows, err := a.pool.Query(ctx, `
SELECT s.worker_id, s.hourly_rate::text, count(*), COALESCE(sum(s.worked_seconds), 0),
       COALESCE(sum(s.calculated_amount)::text, '0.00')
`+timesheetJoinSQL+timesheetFilterSQL+`
GROUP BY s.worker_id, s.hourly_rate
ORDER BY s.worker_id, s.hourly_rate`, filters.args(organizationID)...)
	if err != nil {
		return nil, err
	}
	defer rateRows.Close()
	for rateRows.Next() {
		var workerID string
		var rate rateSummary
		if err := rateRows.Scan(&workerID, &rate.HourlyRate, &rate.Sessions, &rate.WorkedSeconds, &rate.EstimatedPay); err != nil {
			return nil, err
		}
		if index, ok := indexes[workerID]; ok {
			summaries[index].Rates = append(summaries[index].Rates, rate)
		}
	}
	if err := rateRows.Err(); err != nil {
		return nil, err
	}
	return summaries, nil
}

func workedTime(seconds *int64) string {
	if seconds == nil {
		return ""
	}
	value := *seconds
	if value < 0 {
		return ""
	}
	return fmt.Sprintf("%dh%02dm", value/3600, (value%3600)/60)
}

func reportTimestamp(value time.Time, location *time.Location) string {
	return value.In(location).Format(time.RFC3339)
}

func csvValue(value string) string {
	if value == "" {
		return value
	}
	switch value[0] {
	case '=', '+', '-', '@':
		return "'" + value
	default:
		return value
	}
}
