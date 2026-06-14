package customer

import (
	"context"
	"strings"
	"time"
)

// CustomerProfileSummary represents a full customer profile row.
type CustomerProfileSummary struct {
	CustomerID              string    `json:"customerId"`
	CustomerEmailHash       string    `json:"customerEmailHash"`
	FirstName               string    `json:"firstName"`
	LastName                string    `json:"lastName"`
	LoyaltyTier             string    `json:"loyaltyTier"`
	EnrollmentStatus        string    `json:"enrollmentStatus"`
	EnrollmentTransactionID string    `json:"enrollmentTransactionId"`
	PasswordChangeStatus    string    `json:"passwordChangeStatus"`
	PasswordChangeRequestID string    `json:"passwordChangeRequestId"`
	LastLoginID             string    `json:"lastLoginId"`
	LastLoginAt             time.Time `json:"lastLoginAt"`
	Source                  string    `json:"source"`
	Stage                   string    `json:"stage"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

// CustomerByHashResponse is the lightweight response for email-hash lookup.
type CustomerByHashResponse struct {
	CustomerID        string `json:"customerId"`
	CustomerEmailHash string `json:"customerEmailHash"`
	LoyaltyTier       string `json:"loyaltyTier"`
	EnrollmentStatus  string `json:"enrollmentStatus"`
}

// ICustomerRepository abstracts the persistence layer for customer data.
type ICustomerRepository interface {
	GetByEmailHash(ctx context.Context, emailHash string) (*CustomerByHashResponse, error)
	GetProfileSummary(ctx context.Context, customerID string) (*CustomerProfileSummary, error)
}

// CustomerIDFromProfileSummaryPath extracts the customerId from a
// /v1/customers/:customerId/profile-summary path.
func CustomerIDFromProfileSummaryPath(path string) (string, bool) {
	const prefix = "/v1/customers/"
	const suffix = "/profile-summary"

	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}

	customerID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	customerID = strings.Trim(customerID, "/")
	if customerID == "" || strings.Contains(customerID, "/") {
		return "", false
	}

	return customerID, true
}
