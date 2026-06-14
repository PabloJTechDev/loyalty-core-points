package enrollment

import (
	"context"
	"time"
)

// EnrollmentTrace represents a customer enrollment event stored in the DB.
type EnrollmentTrace struct {
	TransactionID     string    `json:"transactionId"`
	CustomerEmailHash string    `json:"customerEmailHash"`
	ReceivedAt        time.Time `json:"receivedAt"`
	Source            string    `json:"source"`
	Stage             string    `json:"stage"`
}

// EnrollmentListResponse is the paginated list response.
type EnrollmentListResponse struct {
	Total int               `json:"total"`
	Items []EnrollmentTrace `json:"items"`
}

// CreateEnrollmentInput holds the fields required to create an enrollment.
type CreateEnrollmentInput struct {
	TransactionID     string
	CustomerEmailHash string
	RemoteAddr        string
}

// IEnrollmentRepository abstracts the persistence layer.
type IEnrollmentRepository interface {
	List(ctx context.Context) ([]EnrollmentTrace, error)
	GetByTransactionID(ctx context.Context, txID string) (*EnrollmentTrace, error)
	Create(ctx context.Context, input CreateEnrollmentInput) (*EnrollmentTrace, error)
}
