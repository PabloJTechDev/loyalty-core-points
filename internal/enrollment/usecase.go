package enrollment

import "context"

// ListEnrollmentsUseCase returns all enrollment traces.
type ListEnrollmentsUseCase struct{ repo IEnrollmentRepository }

func NewListEnrollmentsUseCase(repo IEnrollmentRepository) *ListEnrollmentsUseCase {
	return &ListEnrollmentsUseCase{repo: repo}
}

func (uc *ListEnrollmentsUseCase) Execute(ctx context.Context) ([]EnrollmentTrace, error) {
	return uc.repo.List(ctx)
}

// GetEnrollmentUseCase retrieves a single enrollment by transactionId.
type GetEnrollmentUseCase struct{ repo IEnrollmentRepository }

func NewGetEnrollmentUseCase(repo IEnrollmentRepository) *GetEnrollmentUseCase {
	return &GetEnrollmentUseCase{repo: repo}
}

func (uc *GetEnrollmentUseCase) Execute(ctx context.Context, txID string) (*EnrollmentTrace, error) {
	return uc.repo.GetByTransactionID(ctx, txID)
}

// CreateEnrollmentUseCase persists a new enrollment.
type CreateEnrollmentUseCase struct{ repo IEnrollmentRepository }

func NewCreateEnrollmentUseCase(repo IEnrollmentRepository) *CreateEnrollmentUseCase {
	return &CreateEnrollmentUseCase{repo: repo}
}

func (uc *CreateEnrollmentUseCase) Execute(ctx context.Context, input CreateEnrollmentInput) (*EnrollmentTrace, error) {
	return uc.repo.Create(ctx, input)
}
