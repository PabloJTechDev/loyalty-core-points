package auth

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// PostgresAuthRepo implements IAuthRepository using pgx.
type PostgresAuthRepo struct {
	db *pgxpool.Pool
}

// NewPostgresAuthRepo constructs a new repo.
func NewPostgresAuthRepo(db *pgxpool.Pool) *PostgresAuthRepo {
	return &PostgresAuthRepo{db: db}
}

// ─── Logins ──────────────────────────────────────────────────────────────────

func (r *PostgresAuthRepo) ListLogins(ctx context.Context) ([]LoginTrace, error) {
	rows, err := r.db.Query(ctx, `SELECT login_id, request_id, transaction_id, customer_email_hash, authenticated_at, source, stage
		FROM customer_login_traces
		ORDER BY authenticated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]LoginTrace, 0)
	for rows.Next() {
		trace, err := scanLogin(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, trace)
	}
	return items, rows.Err()
}

func (r *PostgresAuthRepo) GetLoginByID(ctx context.Context, loginID string) (*LoginTrace, error) {
	row := r.db.QueryRow(ctx, `SELECT login_id, request_id, transaction_id, customer_email_hash, authenticated_at, source, stage
		FROM customer_login_traces
		WHERE login_id = $1`, loginID)

	trace, err := scanLogin(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	return &trace, nil
}

func (r *PostgresAuthRepo) CreateLogin(ctx context.Context, input CreateLoginInput) (*LoginTrace, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO customer_login_traces (
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
		input.LoginID, input.RequestID, input.TransactionID, input.CustomerEmailHash, "bff-points", "authenticated")

	trace, err := scanLogin(row)
	if err != nil {
		return nil, err
	}
	return &trace, nil
}

// ─── Password Changes ─────────────────────────────────────────────────────────

func (r *PostgresAuthRepo) ListPasswordChanges(ctx context.Context) ([]PasswordChangeTrace, error) {
	rows, err := r.db.Query(ctx, `SELECT request_id, transaction_id, customer_email_hash, requested_at, source, stage
		FROM customer_password_change_traces
		ORDER BY requested_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PasswordChangeTrace, 0)
	for rows.Next() {
		trace, err := scanPasswordChange(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, trace)
	}
	return items, rows.Err()
}

func (r *PostgresAuthRepo) GetPasswordChangeByRequestID(ctx context.Context, requestID string) (*PasswordChangeTrace, error) {
	row := r.db.QueryRow(ctx, `SELECT request_id, transaction_id, customer_email_hash, requested_at, source, stage
		FROM customer_password_change_traces
		WHERE request_id = $1`, requestID)

	trace, err := scanPasswordChange(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	return &trace, nil
}

func (r *PostgresAuthRepo) CreatePasswordChange(ctx context.Context, input CreatePasswordChangeInput) (*PasswordChangeTrace, error) {
	row := r.db.QueryRow(ctx, `INSERT INTO customer_password_change_traces (
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
		input.RequestID, input.TransactionID, input.CustomerEmailHash, "bff-points", "password_change_requested")

	trace, err := scanPasswordChange(row)
	if err != nil {
		return nil, err
	}
	return &trace, nil
}

// ─── Scan helpers ─────────────────────────────────────────────────────────────

func scanLogin(row shared.RowScanner) (LoginTrace, error) {
	var trace LoginTrace
	err := row.Scan(&trace.LoginID, &trace.RequestID, &trace.TransactionID, &trace.CustomerEmailHash, &trace.AuthenticatedAt, &trace.Source, &trace.Stage)
	return trace, err
}

func scanPasswordChange(row shared.RowScanner) (PasswordChangeTrace, error) {
	var trace PasswordChangeTrace
	err := row.Scan(&trace.RequestID, &trace.TransactionID, &trace.CustomerEmailHash, &trace.RequestedAt, &trace.Source, &trace.Stage)
	return trace, err
}
