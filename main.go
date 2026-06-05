package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const maxBodyBytes = 1024 * 1024

var errPayloadTooLarge = errors.New("payload_too_large")

type logFields map[string]any

func logEvent(event string, fields logFields) {
	payload := map[string]any{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"service": "loyalty-core-points",
		"event":   event,
	}

	for key, value := range fields {
		if value != nil {
			payload[key] = value
		}
	}

	bytes, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("{\"ts\":\"%s\",\"service\":\"loyalty-core-points\",\"event\":\"logger.error\",\"message\":\"json_marshal_failed\"}\n", time.Now().UTC().Format(time.RFC3339))
		return
	}

	fmt.Println(string(bytes))
}

var (
	metricsRegistry       = prometheus.NewRegistry()
	coreHTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loyalty_core_http_requests_total",
			Help: "Total HTTP requests handled by the core service",
		},
		[]string{"method", "route", "status_class", "status_code"},
	)
	coreBusinessTransactionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "loyalty_core_business_transactions_total",
			Help: "Business transactions processed by the core service",
		},
		[]string{"flow", "outcome"},
	)
	coreHTTPRequestDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "loyalty_core_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds for the core service",
			Buckets: []float64{0.01, 0.025, 0.05, 0.1, 0.2, 0.35, 0.5, 0.75, 1, 1.5, 2, 3, 5},
		},
		[]string{"method", "route", "status_class", "status_code"},
	)
)

func init() {
	metricsRegistry.MustRegister(
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewGoCollector(),
		coreHTTPRequestsTotal,
		coreBusinessTransactionsTotal,
		coreHTTPRequestDurationSeconds,
	)
}

type app struct {
	db *pgxpool.Pool
}

type healthResponse struct {
	Status                  string `json:"status"`
	Service                 string `json:"service"`
	Storage                 string `json:"storage"`
	ReceivedTransactions    int32  `json:"receivedTransactions"`
	ReceivedPasswordChanges int32  `json:"receivedPasswordChanges"`
	ReceivedLogins          int32  `json:"receivedLogins"`
	ReceivedProfiles        int32  `json:"receivedProfiles"`
}

type enrollmentTrace struct {
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	ReceivedAt        time.Time `json:"receivedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

type passwordChangeTrace struct {
	RequestID         string    `json:"requestId"`
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	RequestedAt       time.Time `json:"requestedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

type loginTrace struct {
	LoginID           string    `json:"loginId"`
	RequestID         string    `json:"requestId"`
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	AuthenticatedAt   time.Time `json:"authenticatedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

type enrollmentPayload struct {
	TransactionID     string `json:"transactionId"`
	CustomerEmailHash string `json:"customerEmailHash"`
}

type passwordChangePayload struct {
	RequestID         string `json:"requestId"`
	TransactionID     string `json:"transactionId"`
	CustomerEmailHash string `json:"customerEmailHash"`
}

type loginPayload struct {
	LoginID           string `json:"loginId"`
	RequestID         string `json:"requestId"`
	TransactionID     string `json:"transactionId"`
	CustomerEmailHash string `json:"customerEmailHash"`
}

type enrollmentListResponse struct {
	Total int               `json:"total"`
	Items []enrollmentTrace `json:"items"`
}

type customerProfileSummary struct {
	CustomerID              string    `json:"customerId"`
	CustomerEmailHash       string    `json:"customerEmailHash"`
	FirstName               string    `json:"firstName"`
	LastName                string    `json:"lastName"`
	LoyaltyTier             string    `json:"loyaltyTier"`
	EnrollmentStatus        string    `json:"enrollmentStatus"`
	EnrollmentTransactionID string    `json:"enrollmentTransactionId"`
	PasswordChangeStatus    string    `json:"passwordChangeStatus"`
	PasswordChangeRequestID string    `json:"passwordChangeRequestId"`
	LastLoginID             string    `json:"lastLoginId"`
	LastLoginAt             time.Time `json:"lastLoginAt"`
	Source                  string    `json:"source"`
	Stage                   string    `json:"stage"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func main() {
	ctx := context.Background()
	databaseURL, err := mustEnv("DATABASE_URL")
	if err != nil {
		log.Fatalf("Failed to start core-points: %v", err)
	}

	port := getPort()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	app := &app{db: pool}
	if err := app.initDB(ctx); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logEvent("service.started", logFields{
			"port":    port,
			"storage": "postgres",
		})
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown error: %v", err)
	}
}

func mustEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func getPort() int {
	portValue := strings.TrimSpace(os.Getenv("PORT"))
	if portValue == "" {
		return 3001
	}

	port, err := strconv.Atoi(portValue)
	if err != nil || port <= 0 {
		log.Printf("invalid PORT %q, falling back to 3001", portValue)
		return 3001
	}

	return port
}

func normalizeRoute(path string) string {
	switch {
	case strings.HasPrefix(path, "/v1/customer-enrollments/"):
		return "/v1/customer-enrollments/:transactionId"
	case strings.HasPrefix(path, "/v1/customer-password-changes/"):
		return "/v1/customer-password-changes/:requestId"
	case strings.HasPrefix(path, "/v1/customer-logins/"):
		return "/v1/customer-logins/:loginId"
	case strings.HasPrefix(path, "/v1/customers/") && strings.HasSuffix(path, "/profile-summary"):
		return "/v1/customers/:customerId/profile-summary"
	default:
		return path
	}
}

func statusClass(statusCode int) string {
	return fmt.Sprintf("%dxx", statusCode/100)
}

type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *metricsResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}).ServeHTTP)
	mux.HandleFunc("/v1/customer-enrollments", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			a.handleListEnrollments(w, r)
		case http.MethodPost:
			a.handleCreateEnrollment(w, r)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		}
	})
	mux.HandleFunc("/v1/customer-enrollments/", a.handleGetEnrollment)
	mux.HandleFunc("/v1/customer-password-changes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			a.handleCreatePasswordChange(w, r)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		}
	})
	mux.HandleFunc("/v1/customer-password-changes/", a.handleGetPasswordChange)
	mux.HandleFunc("/v1/customer-logins", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			a.handleCreateLogin(w, r)
		default:
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		}
	})
	mux.HandleFunc("/v1/customer-logins/", a.handleGetLogin)
	mux.HandleFunc("/v1/customers/", a.handleGetCustomerProfileSummary)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		wrapped := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		mux.ServeHTTP(wrapped, r)

		route := normalizeRoute(r.URL.Path)
		statusCode := wrapped.statusCode
		labels := prometheus.Labels{
			"method":       r.Method,
			"route":        route,
			"status_class": statusClass(statusCode),
			"status_code":  strconv.Itoa(statusCode),
		}
		coreHTTPRequestsTotal.With(labels).Inc()
		coreHTTPRequestDurationSeconds.With(labels).Observe(time.Since(startedAt).Seconds())
	})
}

func (a *app) initDB(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS customer_enrollment_traces (
			transaction_id TEXT PRIMARY KEY,
			customer_email_hash TEXT NOT NULL,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source TEXT NOT NULL,
			stage TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS customer_password_change_traces (
			request_id TEXT PRIMARY KEY,
			transaction_id TEXT NOT NULL,
			customer_email_hash TEXT NOT NULL,
			requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source TEXT NOT NULL,
			stage TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS customer_login_traces (
			login_id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			transaction_id TEXT NOT NULL,
			customer_email_hash TEXT NOT NULL,
			authenticated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source TEXT NOT NULL,
			stage TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS customer_profiles (
			customer_id TEXT PRIMARY KEY,
			customer_email_hash TEXT NOT NULL,
			first_name TEXT NOT NULL,
			last_name TEXT NOT NULL,
			loyalty_tier TEXT NOT NULL,
			enrollment_status TEXT NOT NULL,
			enrollment_transaction_id TEXT NOT NULL,
			password_change_status TEXT NOT NULL,
			password_change_request_id TEXT NOT NULL,
			last_login_id TEXT NOT NULL,
			last_login_at TIMESTAMPTZ NOT NULL,
			source TEXT NOT NULL,
			stage TEXT NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`INSERT INTO customer_profiles (
			customer_id,
			customer_email_hash,
			first_name,
			last_name,
			loyalty_tier,
			enrollment_status,
			enrollment_transaction_id,
			password_change_status,
			password_change_request_id,
			last_login_id,
			last_login_at,
			source,
			stage,
			updated_at
		) VALUES (
			'cust_001',
			'hash_cust_001_demo',
			'Pablo',
			'Velasquez',
			'gold',
			'enrolled',
			'tx-demo-001',
			'completed',
			'pwd-demo-001',
			'login-demo-001',
			TIMESTAMPTZ '2026-06-03T18:45:00Z',
			'core-points',
			'profile_summary_ready',
			TIMESTAMPTZ '2026-06-03T18:45:00Z'
		)
		ON CONFLICT (customer_id) DO NOTHING`,
	}

	for _, statement := range statements {
		if _, err := a.db.Exec(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	response := healthResponse{
		Status:  "ok",
		Service: "core-points",
		Storage: "postgres",
	}

	queries := []struct {
		query  string
		target *int32
	}{
		{query: `SELECT COUNT(*)::int FROM customer_enrollment_traces`, target: &response.ReceivedTransactions},
		{query: `SELECT COUNT(*)::int FROM customer_password_change_traces`, target: &response.ReceivedPasswordChanges},
		{query: `SELECT COUNT(*)::int FROM customer_login_traces`, target: &response.ReceivedLogins},
		{query: `SELECT COUNT(*)::int FROM customer_profiles`, target: &response.ReceivedProfiles},
	}

	for _, item := range queries {
		if err := a.db.QueryRow(ctx, item.query).Scan(item.target); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"service": "core-points",
				"message": "database_unavailable",
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *app) handleListEnrollments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := a.db.Query(ctx, `SELECT transaction_id, customer_email_hash, received_at, source, stage
		FROM customer_enrollment_traces
		ORDER BY received_at DESC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}
	defer rows.Close()

	items := make([]enrollmentTrace, 0)
	for rows.Next() {
		trace, err := scanEnrollment(rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
			return
		}
		items = append(items, trace)
	}

	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, enrollmentListResponse{Total: len(items), Items: items})
}

func (a *app) handleGetEnrollment(w http.ResponseWriter, r *http.Request) {
	transactionID := strings.TrimPrefix(r.URL.Path, "/v1/customer-enrollments/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := a.db.QueryRow(ctx, `SELECT transaction_id, customer_email_hash, received_at, source, stage
		FROM customer_enrollment_traces
		WHERE transaction_id = $1`, transactionID)

	trace, err := scanEnrollment(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "transactionId": transactionID})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, trace)
}

func (a *app) handleCreateEnrollment(w http.ResponseWriter, r *http.Request) {
	var payload enrollmentPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w, err)
		return
	}

	if payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	logEvent("enrollment.received", logFields{
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	row := a.db.QueryRow(ctx, `INSERT INTO customer_enrollment_traces (
			transaction_id,
			customer_email_hash,
			source,
			stage
		) VALUES ($1, $2, $3, $4)
		ON CONFLICT (transaction_id)
		DO UPDATE SET
			customer_email_hash = EXCLUDED.customer_email_hash,
			source = EXCLUDED.source,
			stage = EXCLUDED.stage
		RETURNING transaction_id, customer_email_hash, received_at, source, stage`,
		payload.TransactionID, payload.CustomerEmailHash, "bff-points", "core_received")

	trace, err := scanEnrollment(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	logEvent("enrollment.stored", logFields{
		"transactionId": trace.TransactionID,
		"stage":         trace.Stage,
		"source":        trace.Source,
		"receivedAt":    trace.ReceivedAt.Format(time.RFC3339),
	})
	coreBusinessTransactionsTotal.WithLabelValues("enrollment", "accepted").Inc()

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":        "accepted",
		"transactionId": trace.TransactionID,
		"receivedAt":    trace.ReceivedAt,
		"storage":       "postgres",
	})
}

func (a *app) handleGetPasswordChange(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimPrefix(r.URL.Path, "/v1/customer-password-changes/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := a.db.QueryRow(ctx, `SELECT request_id, transaction_id, customer_email_hash, requested_at, source, stage
		FROM customer_password_change_traces
		WHERE request_id = $1`, requestID)

	trace, err := scanPasswordChange(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "requestId": requestID})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, trace)
}

func (a *app) handleCreatePasswordChange(w http.ResponseWriter, r *http.Request) {
	var payload passwordChangePayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w, err)
		return
	}

	if payload.RequestID == "" || payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "requestId, transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	logEvent("password-change.received", logFields{
		"requestId":         payload.RequestID,
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	row := a.db.QueryRow(ctx, `INSERT INTO customer_password_change_traces (
			request_id,
			transaction_id,
			customer_email_hash,
			source,
			stage
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (request_id)
		DO UPDATE SET
			transaction_id = EXCLUDED.transaction_id,
			customer_email_hash = EXCLUDED.customer_email_hash,
			source = EXCLUDED.source,
			stage = EXCLUDED.stage
		RETURNING request_id, transaction_id, customer_email_hash, requested_at, source, stage`,
		payload.RequestID, payload.TransactionID, payload.CustomerEmailHash, "bff-points", "password_change_requested")

	trace, err := scanPasswordChange(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	logEvent("password-change.stored", logFields{
		"requestId":     trace.RequestID,
		"transactionId": trace.TransactionID,
		"stage":         trace.Stage,
		"source":        trace.Source,
		"requestedAt":   trace.RequestedAt.Format(time.RFC3339),
	})
	coreBusinessTransactionsTotal.WithLabelValues("password_change", "accepted").Inc()

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":        "accepted",
		"requestId":     trace.RequestID,
		"transactionId": trace.TransactionID,
		"requestedAt":   trace.RequestedAt,
		"storage":       "postgres",
	})
}

func (a *app) handleGetLogin(w http.ResponseWriter, r *http.Request) {
	loginID := strings.TrimPrefix(r.URL.Path, "/v1/customer-logins/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	row := a.db.QueryRow(ctx, `SELECT login_id, request_id, transaction_id, customer_email_hash, authenticated_at, source, stage
		FROM customer_login_traces
		WHERE login_id = $1`, loginID)

	trace, err := scanLogin(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "loginId": loginID})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	writeJSON(w, http.StatusOK, trace)
}

func (a *app) handleCreateLogin(w http.ResponseWriter, r *http.Request) {
	var payload loginPayload
	if err := decodeJSONBody(r, &payload); err != nil {
		writeBadRequest(w, err)
		return
	}

	if payload.LoginID == "" || payload.RequestID == "" || payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "loginId, requestId, transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	logEvent("login.received", logFields{
		"loginId":           payload.LoginID,
		"requestId":         payload.RequestID,
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	row := a.db.QueryRow(ctx, `INSERT INTO customer_login_traces (
			login_id,
			request_id,
			transaction_id,
			customer_email_hash,
			source,
			stage
		) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (login_id)
		DO UPDATE SET
			request_id = EXCLUDED.request_id,
			transaction_id = EXCLUDED.transaction_id,
			customer_email_hash = EXCLUDED.customer_email_hash,
			source = EXCLUDED.source,
			stage = EXCLUDED.stage
		RETURNING login_id, request_id, transaction_id, customer_email_hash, authenticated_at, source, stage`,
		payload.LoginID, payload.RequestID, payload.TransactionID, payload.CustomerEmailHash, "bff-points", "authenticated")

	trace, err := scanLogin(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	logEvent("login.stored", logFields{
		"loginId":         trace.LoginID,
		"requestId":       trace.RequestID,
		"transactionId":   trace.TransactionID,
		"stage":           trace.Stage,
		"source":          trace.Source,
		"authenticatedAt": trace.AuthenticatedAt.Format(time.RFC3339),
	})
	coreBusinessTransactionsTotal.WithLabelValues("login", "accepted").Inc()

	writeJSON(w, http.StatusCreated, map[string]any{
		"status":          "accepted",
		"loginId":         trace.LoginID,
		"requestId":       trace.RequestID,
		"transactionId":   trace.TransactionID,
		"authenticatedAt": trace.AuthenticatedAt,
		"storage":         "postgres",
	})
}

func (a *app) handleGetCustomerProfileSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		return
	}

	customerID, ok := customerIDFromProfileSummaryPath(r.URL.Path)
	if !ok || strings.TrimSpace(customerID) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	logEvent("profile-summary.requested", logFields{
		"customerId": customerID,
		"remote":     r.RemoteAddr,
	})

	row := a.db.QueryRow(ctx, `SELECT
			customer_id,
			customer_email_hash,
			first_name,
			last_name,
			loyalty_tier,
			enrollment_status,
			enrollment_transaction_id,
			password_change_status,
			password_change_request_id,
			last_login_id,
			last_login_at,
			source,
			stage,
			updated_at
		FROM customer_profiles
		WHERE customer_id = $1`, customerID)

	summary, err := scanCustomerProfileSummary(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logEvent("profile-summary.not-found", logFields{"customerId": customerID})
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "customerId": customerID})
			return
		}
		logEvent("profile-summary.error", logFields{"customerId": customerID, "message": "database_unavailable"})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	logEvent("profile-summary.found", logFields{
		"customerId":  summary.CustomerID,
		"loyaltyTier": summary.LoyaltyTier,
		"stage":       summary.Stage,
		"updatedAt":   summary.UpdatedAt.Format(time.RFC3339),
	})
	coreBusinessTransactionsTotal.WithLabelValues("profile_summary", "served").Inc()

	writeJSON(w, http.StatusOK, summary)
}

func scanEnrollment(row rowScanner) (enrollmentTrace, error) {
	var trace enrollmentTrace
	err := row.Scan(&trace.TransactionID, &trace.CustomerEmailHash, &trace.ReceivedAt, &trace.Source, &trace.Stage)
	return trace, err
}

func scanPasswordChange(row rowScanner) (passwordChangeTrace, error) {
	var trace passwordChangeTrace
	err := row.Scan(&trace.RequestID, &trace.TransactionID, &trace.CustomerEmailHash, &trace.RequestedAt, &trace.Source, &trace.Stage)
	return trace, err
}

func scanLogin(row rowScanner) (loginTrace, error) {
	var trace loginTrace
	err := row.Scan(&trace.LoginID, &trace.RequestID, &trace.TransactionID, &trace.CustomerEmailHash, &trace.AuthenticatedAt, &trace.Source, &trace.Stage)
	return trace, err
}

func scanCustomerProfileSummary(row rowScanner) (customerProfileSummary, error) {
	var summary customerProfileSummary
	err := row.Scan(
		&summary.CustomerID,
		&summary.CustomerEmailHash,
		&summary.FirstName,
		&summary.LastName,
		&summary.LoyaltyTier,
		&summary.EnrollmentStatus,
		&summary.EnrollmentTransactionID,
		&summary.PasswordChangeStatus,
		&summary.PasswordChangeRequestID,
		&summary.LastLoginID,
		&summary.LastLoginAt,
		&summary.Source,
		&summary.Stage,
		&summary.UpdatedAt,
	)
	return summary, err
}

func customerIDFromProfileSummaryPath(path string) (string, bool) {
	const prefix = "/v1/customers/"
	const suffix = "/profile-summary"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}

	customerID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	customerID = strings.Trim(customerID, "/")
	if customerID == "" || strings.Contains(customerID, "/") {
		return "", false
	}

	return customerID, true
}

func decodeJSONBody(r *http.Request, target any) error {
	limitedBody := io.LimitReader(r.Body, maxBodyBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if len(body) > maxBodyBytes {
		return errPayloadTooLarge
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("{}")
	}

	if err := json.Unmarshal(body, target); err != nil {
		return err
	}

	return nil
}

func writeBadRequest(w http.ResponseWriter, err error) {
	message := "invalid json payload"
	if errors.Is(err, errPayloadTooLarge) {
		message = "payload too large"
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{
		"status":  "error",
		"message": message,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}
