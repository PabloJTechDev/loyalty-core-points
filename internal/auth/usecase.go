package auth

import "context"

// ─── Login use cases ─────────────────────────────────────────────────────────

type ListLoginsUseCase struct{ repo IAuthRepository }

func NewListLoginsUseCase(repo IAuthRepository) *ListLoginsUseCase {
	return &ListLoginsUseCase{repo: repo}
}

func (uc *ListLoginsUseCase) Execute(ctx context.Context) ([]LoginTrace, error) {
	return uc.repo.ListLogins(ctx)
}

type GetLoginUseCase struct{ repo IAuthRepository }

func NewGetLoginUseCase(repo IAuthRepository) *GetLoginUseCase {
	return &GetLoginUseCase{repo: repo}
}

func (uc *GetLoginUseCase) Execute(ctx context.Context, loginID string) (*LoginTrace, error) {
	return uc.repo.GetLoginByID(ctx, loginID)
}

type CreateLoginUseCase struct{ repo IAuthRepository }

func NewCreateLoginUseCase(repo IAuthRepository) *CreateLoginUseCase {
	return &CreateLoginUseCase{repo: repo}
}

func (uc *CreateLoginUseCase) Execute(ctx context.Context, input CreateLoginInput) (*LoginTrace, error) {
	return uc.repo.CreateLogin(ctx, input)
}

// ─── PasswordChange use cases ─────────────────────────────────────────────────

type ListPasswordChangesUseCase struct{ repo IAuthRepository }

func NewListPasswordChangesUseCase(repo IAuthRepository) *ListPasswordChangesUseCase {
	return &ListPasswordChangesUseCase{repo: repo}
}

func (uc *ListPasswordChangesUseCase) Execute(ctx context.Context) ([]PasswordChangeTrace, error) {
	return uc.repo.ListPasswordChanges(ctx)
}

type GetPasswordChangeUseCase struct{ repo IAuthRepository }

func NewGetPasswordChangeUseCase(repo IAuthRepository) *GetPasswordChangeUseCase {
	return &GetPasswordChangeUseCase{repo: repo}
}

func (uc *GetPasswordChangeUseCase) Execute(ctx context.Context, requestID string) (*PasswordChangeTrace, error) {
	return uc.repo.GetPasswordChangeByRequestID(ctx, requestID)
}

type CreatePasswordChangeUseCase struct{ repo IAuthRepository }

func NewCreatePasswordChangeUseCase(repo IAuthRepository) *CreatePasswordChangeUseCase {
	return &CreatePasswordChangeUseCase{repo: repo}
}

func (uc *CreatePasswordChangeUseCase) Execute(ctx context.Context, input CreatePasswordChangeInput) (*PasswordChangeTrace, error) {
	return uc.repo.CreatePasswordChange(ctx, input)
}
