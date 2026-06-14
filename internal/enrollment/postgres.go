package enrollment

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// PostgresEnrollmentRepo implements IEnrollmentRepository using pgx.
type PostgresEnrollmentRepo struct {
	db *pgxpool.Pool
}

// NewPostgresEnrollmentRepo constructs a new repo.
func NewPostgresEnrollmentRepo(db *pgxpool.Pool) *PostgresEnrollmentRepo {
	return &PostgresEnrollmentRepo{db: db}
}

func (r *PostgresEnrollmentRepo) List(ctx context.Context) ([]EnrollmentTrace, error) {
	rows, err := r.db.Query(ctx, `SELECT transaction_id, customer_email_hash, received_at, source, stage
		FROM customer_enrollment_traces
		ORDER BY received_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]EnrollmentTrace, 0)
	for rows.Next() {
		trace, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, trace)
	}
	return items, rows.Err()
}

func (r *PostgresEnrollmentRepo) GetByTransactionID(ctx context.Context, txID string) (*EnrollmentTrace, error) {
	row := r.db.QueryRow(ctx, `SELECT transaction_id, customer_email_hash, received_at, source, stage
		FROM customer_enrollment_traces
		WHERE transaction_id = $1`, txID)

	trace, err := scanEnrollment(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	return &trace, nil
}

func (r *PostgresEnrollmentRepo) Create(ctx context.Context, input CreateEnrollmentInput) (*EnrollmentTrace, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO customer_enrollment_traces (
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
		input.TransactionID, input.CustomerEmailHash, "bff-points", "core_received")

	trace, err := scanEnrollment(row)
	if err != nil {
		return nil, err
	}

	// Auto-upsert customer profile with deterministic customerId
	customerID := deriveCustomerID(input.CustomerEmailHash)
	_, _ = r.db.Exec(ctx, `INSERT INTO customer_profiles (
			customer_id, customer_email_hash, first_name, last_name,
			loyalty_tier, enrollment_status, enrollment_transaction_id,
			password_change_status, password_change_request_id,
			last_login_id, last_login_at, source, stage, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), $11, $12, NOW())
		ON CONFLICT (customer_id) DO UPDATE SET
			enrollment_status = 'enrolled',
			enrollment_transaction_id = EXCLUDED.enrollment_transaction_id,
			updated_at = NOW()`,
		customerID, input.CustomerEmailHash, "Customer", "",
		"silver", "enrolled", input.TransactionID,
		"pending", "",
		"enrollment", "bff-points", "profile_created")

	return &trace, nil
}

// deriveCustomerID produces a deterministic customerId from an email hash.
func deriveCustomerID(emailHash string) string {
	if len(emailHash) >= 12 {
		return "cust_" + emailHash[:12]
	}
	return "cust_" + emailHash
}

func scanEnrollment(row shared.RowScanner) (EnrollmentTrace, error) {
	var trace EnrollmentTrace
	err := row.Scan(&trace.TransactionID, &trace.CustomerEmailHash, &trace.ReceivedAt, &trace.Source, &trace.Stage)
	return trace, err
}
