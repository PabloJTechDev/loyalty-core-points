package points

import (
	"context"
	"time"
)

// PointBalanceResponse represents the current balance of a customer account.
type PointBalanceResponse struct {
	CustomerID       string    `json:"customerId"`
	BalancePoints    int       `json:"balancePoints"`
	LifetimeAccrued  int       `json:"lifetimeAccrued"`
	LifetimeRedeemed int       `json:"lifetimeRedeemed"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// PointTransactionResponse represents a single point ledger entry.
type PointTransactionResponse struct {
	TransactionID string `json:"transactionId"`
	CustomerID    string `json:"customerId"`
	Type          string `json:"type"`
	Points        int    `json:"points"`
	ReferenceID   string `json:"referenceId"`
	Source        string `json:"source"`
	Description   string `json:"description"`
	CreatedAt     string `json:"createdAt"`
}

// AccrueInput is the payload for accruing points.
type AccrueInput struct {
	CustomerID  string
	Points      int
	ReferenceID string
	Source      string
	Description string
}

// RedeemInput is the payload for redeeming points.
type RedeemInput struct {
	CustomerID  string
	Points      int
	ReferenceID string
	Source      string
	Description string
}

// AccrueResult is the result of a successful accrue operation.
type AccrueResult struct {
	TransactionID string
	Accrued       int
}

// RedeemResult is the result of a successful redeem operation.
type RedeemResult struct {
	TransactionID    string
	Redeemed         int
	RemainingBalance int
}

// StatsResult contains aggregated points statistics.
type StatsResult struct {
	TotalPointsInCirculation int64
	TotalLifetimeAccrued     int64
	TotalLifetimeRedeemed    int64
	TotalActiveAccounts      int32
}

// IPointsRepository abstracts the persistence layer for points operations.
type IPointsRepository interface {
	Accrue(ctx context.Context, input AccrueInput) (*AccrueResult, error)
	Redeem(ctx context.Context, input RedeemInput) (*RedeemResult, error)
	GetBalance(ctx context.Context, customerID string) (*PointBalanceResponse, error)
	GetTransactions(ctx context.Context, customerID string) ([]PointTransactionResponse, error)
	GetStats(ctx context.Context) (*StatsResult, error)
}
