package points

import "context"

// AccrueUseCase accrues points for a customer.
type AccrueUseCase struct{ repo IPointsRepository }

func NewAccrueUseCase(repo IPointsRepository) *AccrueUseCase {
	return &AccrueUseCase{repo: repo}
}

func (uc *AccrueUseCase) Execute(ctx context.Context, input AccrueInput) (*AccrueResult, error) {
	return uc.repo.Accrue(ctx, input)
}

// RedeemUseCase redeems points for a customer.
type RedeemUseCase struct{ repo IPointsRepository }

func NewRedeemUseCase(repo IPointsRepository) *RedeemUseCase {
	return &RedeemUseCase{repo: repo}
}

func (uc *RedeemUseCase) Execute(ctx context.Context, input RedeemInput) (*RedeemResult, error) {
	return uc.repo.Redeem(ctx, input)
}

// GetBalanceUseCase retrieves the current balance for a customer.
type GetBalanceUseCase struct{ repo IPointsRepository }

func NewGetBalanceUseCase(repo IPointsRepository) *GetBalanceUseCase {
	return &GetBalanceUseCase{repo: repo}
}

func (uc *GetBalanceUseCase) Execute(ctx context.Context, customerID string) (*PointBalanceResponse, error) {
	return uc.repo.GetBalance(ctx, customerID)
}

// GetTransactionsUseCase retrieves point transactions for a customer.
type GetTransactionsUseCase struct{ repo IPointsRepository }

func NewGetTransactionsUseCase(repo IPointsRepository) *GetTransactionsUseCase {
	return &GetTransactionsUseCase{repo: repo}
}

func (uc *GetTransactionsUseCase) Execute(ctx context.Context, customerID string) ([]PointTransactionResponse, error) {
	return uc.repo.GetTransactions(ctx, customerID)
}

// GetStatsUseCase retrieves aggregate points statistics.
type GetStatsUseCase struct{ repo IPointsRepository }

func NewGetStatsUseCase(repo IPointsRepository) *GetStatsUseCase {
	return &GetStatsUseCase{repo: repo}
}

func (uc *GetStatsUseCase) Execute(ctx context.Context) (*StatsResult, error) {
	return uc.repo.GetStats(ctx)
}
