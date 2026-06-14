package customer

import "context"

// GetByEmailHashUseCase retrieves a customer by email hash.
type GetByEmailHashUseCase struct{ repo ICustomerRepository }

func NewGetByEmailHashUseCase(repo ICustomerRepository) *GetByEmailHashUseCase {
	return &GetByEmailHashUseCase{repo: repo}
}

func (uc *GetByEmailHashUseCase) Execute(ctx context.Context, emailHash string) (*CustomerByHashResponse, error) {
	return uc.repo.GetByEmailHash(ctx, emailHash)
}

// GetProfileSummaryUseCase retrieves the full profile summary for a customer.
type GetProfileSummaryUseCase struct{ repo ICustomerRepository }

func NewGetProfileSummaryUseCase(repo ICustomerRepository) *GetProfileSummaryUseCase {
	return &GetProfileSummaryUseCase{repo: repo}
}

func (uc *GetProfileSummaryUseCase) Execute(ctx context.Context, customerID string) (*CustomerProfileSummary, error) {
	return uc.repo.GetProfileSummary(ctx, customerID)
}
