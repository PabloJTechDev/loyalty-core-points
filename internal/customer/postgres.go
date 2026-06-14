package customer

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// PostgresCustomerRepo implements ICustomerRepository using pgx.
type PostgresCustomerRepo struct {
	db *pgxpool.Pool
}

// NewPostgresCustomerRepo constructs a new repo.
func NewPostgresCustomerRepo(db *pgxpool.Pool) *PostgresCustomerRepo {
	return &PostgresCustomerRepo{db: db}
}

func (r *PostgresCustomerRepo) GetByEmailHash(ctx context.Context, emailHash string) (*CustomerByHashResponse, error) {
	var resp CustomerByHashResponse
	err := r.db.QueryRow(ctx, `SELECT customer_id, customer_email_hash, loyalty_tier, enrollment_status
		FROM customer_profiles WHERE customer_email_hash = $1`, emailHash).Scan(
		&resp.CustomerID, &resp.CustomerEmailHash, &resp.LoyaltyTier, &resp.EnrollmentStatus,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	return &resp, nil
}

func (r *PostgresCustomerRepo) GetProfileSummary(ctx context.Context, customerID string) (*CustomerProfileSummary, error) {
	row := r.db.QueryRow(ctx, `SELECT
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
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	return &summary, nil
}

func scanCustomerProfileSummary(row shared.RowScanner) (CustomerProfileSummary, error) {
	var s CustomerProfileSummary
	err := row.Scan(
		&s.CustomerID,
		&s.CustomerEmailHash,
		&s.FirstName,
		&s.LastName,
		&s.LoyaltyTier,
		&s.EnrollmentStatus,
		&s.EnrollmentTransactionID,
		&s.PasswordChangeStatus,
		&s.PasswordChangeRequestID,
		&s.LastLoginID,
		&s.LastLoginAt,
		&s.Source,
		&s.Stage,
		&s.UpdatedAt,
	)
	return s, err
}
