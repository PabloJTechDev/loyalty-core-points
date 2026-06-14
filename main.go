package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/PabloJTechDev/loyalty-core-points/internal/auth"
	"github.com/PabloJTechDev/loyalty-core-points/internal/customer"
	"github.com/PabloJTechDev/loyalty-core-points/internal/enrollment"
	"github.com/PabloJTechDev/loyalty-core-points/internal/points"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

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

type pointsStatsResponse struct {
	TotalEnrollments         int32 `json:"totalEnrollments"`
	TotalPasswordChanges     int32 `json:"totalPasswordChanges"`
	TotalLogins              int32 `json:"totalLogins"`
	PendingEnrollments       int32 `json:"pendingEnrollments"`
	PendingPasswordChanges   int32 `json:"pendingPasswordChanges"`
	TotalPointsInCirculation int64 `json:"totalPointsInCirculation"`
	TotalLifetimeAccrued     int64 `json:"totalLifetimeAccrued"`
	TotalLifetimeRedeemed    int64 `json:"totalLifetimeRedeemed"`
	TotalActiveAccounts      int32 `json:"totalActiveAccounts"`
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
		shared.LogEvent("service.started", shared.LogFields{
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
	case strings.HasPrefix(path, "/v1/customers/by-hash/"):
		return "/v1/customers/by-hash/:emailHash"
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
	// ─── Enrollment ──────────────────────────────────────────────────────────
	enrollmentRepo := enrollment.NewPostgresEnrollmentRepo(a.db)
	enrollmentH := enrollment.NewEnrollmentHandler(
		enrollment.NewListEnrollmentsUseCase(enrollmentRepo),
		enrollment.NewGetEnrollmentUseCase(enrollmentRepo),
		enrollment.NewCreateEnrollmentUseCase(enrollmentRepo),
	)

	// ─── Auth ─────────────────────────────────────────────────────────────────
	authRepo := auth.NewPostgresAuthRepo(a.db)
	authH := auth.NewAuthHandler(
		auth.NewListLoginsUseCase(authRepo),
		auth.NewGetLoginUseCase(authRepo),
		auth.NewCreateLoginUseCase(authRepo),
		auth.NewListPasswordChangesUseCase(authRepo),
		auth.NewGetPasswordChangeUseCase(authRepo),
		auth.NewCreatePasswordChangeUseCase(authRepo),
	)

	// ─── Customer ─────────────────────────────────────────────────────────────
	customerRepo := customer.NewPostgresCustomerRepo(a.db)
	customerH := customer.NewCustomerHandler(
		customer.NewGetByEmailHashUseCase(customerRepo),
		customer.NewGetProfileSummaryUseCase(customerRepo),
	)

	// ─── Points ───────────────────────────────────────────────────────────────
	pointsRepo := points.NewPostgresPointsRepo(a.db)
	pointsH := points.NewPointsHandler(
		points.NewAccrueUseCase(pointsRepo),
		points.NewRedeemUseCase(pointsRepo),
		points.NewGetBalanceUseCase(pointsRepo),
		points.NewGetTransactionsUseCase(pointsRepo),
		points.NewGetStatsUseCase(pointsRepo),
	)

	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/v1/stats", a.handleStats(pointsH))
	mux.HandleFunc("/metrics", promhttp.HandlerFor(shared.MetricsRegistry, promhttp.HandlerOpts{}).ServeHTTP)

	mux.HandleFunc("/v1/customer-enrollments", enrollmentH.HandleEnrollments)
	mux.HandleFunc("/v1/customer-enrollments/", enrollmentH.HandleGetEnrollment)

	mux.HandleFunc("/v1/customer-password-changes", authH.HandlePasswordChanges)
	mux.HandleFunc("/v1/customer-password-changes/", authH.HandleGetPasswordChange)

	mux.HandleFunc("/v1/customer-logins", authH.HandleLogins)
	mux.HandleFunc("/v1/customer-logins/", authH.HandleGetLogin)

	mux.HandleFunc("/v1/customers/by-hash/", customerH.HandleGetCustomerByEmailHash)
	mux.HandleFunc("/v1/customers/", customerH.HandleGetCustomerProfileSummary)

	mux.HandleFunc("/v1/points/accrue", pointsH.HandleAccruePoints)
	mux.HandleFunc("/v1/points/redeem", pointsH.HandleRedeemPoints)
	mux.HandleFunc("/v1/points/", pointsH.HandlePointsRouter)

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
		shared.CoreHTTPRequestsTotal.With(labels).Inc()
		shared.CoreHTTPRequestDurationSeconds.With(labels).Observe(time.Since(startedAt).Seconds())
	})
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
			shared.WriteJSON(w, http.StatusInternalServerError, map[string]any{
				"status":  "error",
				"service": "core-points",
				"message": "database_unavailable",
			})
			return
		}
	}

	shared.WriteJSON(w, http.StatusOK, response)
}

// handleStats returns a closure that combines enrollment/auth counts + points stats.
func (a *app) handleStats(pointsH *points.PointsHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		var stats pointsStatsResponse

		int32Queries := []struct {
			query  string
			target *int32
		}{
			{query: `SELECT COUNT(*)::int FROM customer_enrollment_traces`, target: &stats.TotalEnrollments},
			{query: `SELECT COUNT(*)::int FROM customer_password_change_traces`, target: &stats.TotalPasswordChanges},
			{query: `SELECT COUNT(*)::int FROM customer_login_traces`, target: &stats.TotalLogins},
			{query: `SELECT COUNT(*)::int FROM customer_enrollment_traces WHERE stage = 'pending'`, target: &stats.PendingEnrollments},
			{query: `SELECT COUNT(*)::int FROM customer_password_change_traces WHERE stage = 'pending'`, target: &stats.PendingPasswordChanges},
		}

		for _, item := range int32Queries {
			if err := a.db.QueryRow(ctx, item.query).Scan(item.target); err != nil {
				shared.WriteJSON(w, http.StatusInternalServerError, map[string]any{
					"status":  "error",
					"service": "core-points",
					"message": "database_unavailable",
				})
				return
			}
		}

		extra := map[string]any{
			"totalEnrollments":       stats.TotalEnrollments,
			"totalPasswordChanges":   stats.TotalPasswordChanges,
			"totalLogins":            stats.TotalLogins,
			"pendingEnrollments":     stats.PendingEnrollments,
			"pendingPasswordChanges": stats.PendingPasswordChanges,
		}

		pointsH.HandleStats(w, r, extra)
	}
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
		`CREATE TABLE IF NOT EXISTS point_accounts (
			customer_id TEXT PRIMARY KEY,
			balance_points INTEGER NOT NULL DEFAULT 0,
			lifetime_accrued INTEGER NOT NULL DEFAULT 0,
			lifetime_redeemed INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE TABLE IF NOT EXISTS point_transactions (
			transaction_id TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL,
			type TEXT NOT NULL,
			points INTEGER NOT NULL,
			reference_id TEXT NOT NULL,
			source TEXT NOT NULL,
			description TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_point_transactions_customer_id ON point_transactions(customer_id)`,
	}

	for _, statement := range statements {
		if _, err := a.db.Exec(ctx, statement); err != nil {
			return err
		}
	}

	return nil
}
