package auth

import (
	"context"
	"time"
)

// LoginTrace represents a customer login event.
type LoginTrace struct {
	LoginID           string    `json:"loginId"`
	RequestID         string    `json:"requestId"`
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	AuthenticatedAt   time.Time `json:"authenticatedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

// LoginListResponse is the paginated list response for logins.
type LoginListResponse struct {
	Total int          `json:"total"`
	Items []LoginTrace `json:"items"`
}

// CreateLoginInput holds fields required to create a login trace.
type CreateLoginInput struct {
	LoginID           string
	RequestID         string
	TransactionID     string
	CustomerEmailHash string
}

// PasswordChangeTrace represents a password change event.
type PasswordChangeTrace struct {
	RequestID         string    `json:"requestId"`
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	RequestedAt       time.Time `json:"requestedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

// PasswordChangeListResponse is the paginated list response for password changes.
type PasswordChangeListResponse struct {
	Total int                   `json:"total"`
	Items []PasswordChangeTrace `json:"items"`
}

// CreatePasswordChangeInput holds fields required to create a password change trace.
type CreatePasswordChangeInput struct {
	RequestID         string
	TransactionID     string
	CustomerEmailHash string
}

// IAuthRepository abstracts the persistence layer for auth events.
type IAuthRepository interface {
	ListLogins(ctx context.Context) ([]LoginTrace, error)
	GetLoginByID(ctx context.Context, loginID string) (*LoginTrace, error)
	CreateLogin(ctx context.Context, input CreateLoginInput) (*LoginTrace, error)

	ListPasswordChanges(ctx context.Context) ([]PasswordChangeTrace, error)
	GetPasswordChangeByRequestID(ctx context.Context, requestID string) (*PasswordChangeTrace, error)
	CreatePasswordChange(ctx context.Context, input CreatePasswordChangeInput) (*PasswordChangeTrace, error)
}
