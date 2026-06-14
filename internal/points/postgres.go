package points

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresPointsRepo implements IPointsRepository using pgx.
type PostgresPointsRepo struct {
	db *pgxpool.Pool
}

// NewPostgresPointsRepo constructs a new repo.
func NewPostgresPointsRepo(db *pgxpool.Pool) *PostgresPointsRepo {
	return &PostgresPointsRepo{db: db}
}

func (r *PostgresPointsRepo) Accrue(ctx context.Context, input AccrueInput) (*AccrueResult, error) {
	txID := fmt.Sprintf("ptx_accrue_%s", input.ReferenceID)

	_, err := r.db.Exec(ctx, `
		INSERT INTO point_accounts (customer_id, balance_points, lifetime_accrued, updated_at)
		VALUES ($1, $2, $2, NOW())
		ON CONFLICT (customer_id) DO UPDATE
		SET balance_points   = point_accounts.balance_points + $2,
		    lifetime_accrued = point_accounts.lifetime_accrued + $2,
		    updated_at       = NOW()
	`, input.CustomerID, input.Points)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO point_transactions (transaction_id, customer_id, type, points, reference_id, source, description)
		VALUES ($1, $2, 'accrue', $3, $4, $5, $6)
		ON CONFLICT (transaction_id) DO NOTHING
	`, txID, input.CustomerID, input.Points, input.ReferenceID, input.Source, input.Description)
	if err != nil {
		return nil, err
	}

	return &AccrueResult{TransactionID: txID, Accrued: input.Points}, nil
}

func (r *PostgresPointsRepo) Redeem(ctx context.Context, input RedeemInput) (*RedeemResult, error) {
	txID := fmt.Sprintf("ptx_redeem_%s", input.ReferenceID)

	var balance int
	err := r.db.QueryRow(ctx, `SELECT balance_points FROM point_accounts WHERE customer_id = $1`, input.CustomerID).Scan(&balance)
	if err != nil {
		return nil, fmt.Errorf("customer_account_not_found")
	}
	if balance < input.Points {
		return nil, fmt.Errorf("insufficient_points:%d:%d", balance, input.Points)
	}

	_, err = r.db.Exec(ctx, `
		UPDATE point_accounts
		SET balance_points    = balance_points - $2,
		    lifetime_redeemed = lifetime_redeemed + $2,
		    updated_at        = NOW()
		WHERE customer_id = $1
	`, input.CustomerID, input.Points)
	if err != nil {
		return nil, err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO point_transactions (transaction_id, customer_id, type, points, reference_id, source, description)
		VALUES ($1, $2, 'redeem', $3, $4, $5, $6)
		ON CONFLICT (transaction_id) DO NOTHING
	`, txID, input.CustomerID, input.Points, input.ReferenceID, input.Source, input.Description)
	if err != nil {
		return nil, err
	}

	return &RedeemResult{
		TransactionID:    txID,
		Redeemed:         input.Points,
		RemainingBalance: balance - input.Points,
	}, nil
}

func (r *PostgresPointsRepo) GetBalance(ctx context.Context, customerID string) (*PointBalanceResponse, error) {
	var resp PointBalanceResponse
	resp.CustomerID = customerID
	err := r.db.QueryRow(ctx, `
		SELECT balance_points, lifetime_accrued, lifetime_redeemed, updated_at
		FROM point_accounts WHERE customer_id = $1
	`, customerID).Scan(&resp.BalancePoints, &resp.LifetimeAccrued, &resp.LifetimeRedeemed, &resp.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (r *PostgresPointsRepo) GetTransactions(ctx context.Context, customerID string) ([]PointTransactionResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT transaction_id, customer_id, type, points, reference_id, source, description, created_at
		FROM point_transactions
		WHERE customer_id = $1
		ORDER BY created_at DESC
		LIMIT 50
	`, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []PointTransactionResponse{}
	for rows.Next() {
		var item PointTransactionResponse
		if err := rows.Scan(&item.TransactionID, &item.CustomerID, &item.Type, &item.Points, &item.ReferenceID, &item.Source, &item.Description, &item.CreatedAt); err != nil {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *PostgresPointsRepo) GetStats(ctx context.Context) (*StatsResult, error) {
	result := &StatsResult{}

	int32Queries := []struct {
		query  string
		target *int32
	}{
		{query: `SELECT COUNT(*)::int FROM point_accounts WHERE balance_points > 0`, target: &result.TotalActiveAccounts},
	}
	for _, item := range int32Queries {
		if err := r.db.QueryRow(ctx, item.query).Scan(item.target); err != nil {
			return nil, err
		}
	}

	int64Queries := []struct {
		query  string
		target *int64
	}{
		{query: `SELECT COALESCE(SUM(balance_points), 0) FROM point_accounts`, target: &result.TotalPointsInCirculation},
		{query: `SELECT COALESCE(SUM(lifetime_accrued), 0) FROM point_accounts`, target: &result.TotalLifetimeAccrued},
		{query: `SELECT COALESCE(SUM(lifetime_redeemed), 0) FROM point_accounts`, target: &result.TotalLifetimeRedeemed},
	}
	for _, item := range int64Queries {
		if err := r.db.QueryRow(ctx, item.query).Scan(item.target); err != nil {
			return nil, err
		}
	}

	return result, nil
}
