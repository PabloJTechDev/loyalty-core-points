package customer

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// CustomerHandler handles HTTP requests for the customer journey.
type CustomerHandler struct {
	getByHash      *GetByEmailHashUseCase
	getProfileSummary *GetProfileSummaryUseCase
}

// NewCustomerHandler constructs a CustomerHandler.
func NewCustomerHandler(
	getByHash *GetByEmailHashUseCase,
	getProfileSummary *GetProfileSummaryUseCase,
) *CustomerHandler {
	return &CustomerHandler{
		getByHash:         getByHash,
		getProfileSummary: getProfileSummary,
	}
}

// HandleGetCustomerByEmailHash handles GET /v1/customers/by-hash/:emailHash
func (h *CustomerHandler) HandleGetCustomerByEmailHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
		return
	}

	emailHash := strings.TrimPrefix(r.URL.Path, "/v1/customers/by-hash/")
	emailHash = strings.Trim(emailHash, "/")
	if emailHash == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "message": "emailHash is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp, err := h.getByHash.Execute(ctx, emailHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "emailHash": emailHash})
			return
		}
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, resp)
}

// HandleGetCustomerProfileSummary handles GET /v1/customers/:customerId/profile-summary
func (h *CustomerHandler) HandleGetCustomerProfileSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		return
	}

	customerID, ok := CustomerIDFromProfileSummaryPath(r.URL.Path)
	if !ok || strings.TrimSpace(customerID) == "" {
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	shared.LogEvent("profile-summary.requested", shared.LogFields{
		"customerId": customerID,
		"remote":     r.RemoteAddr,
	})

	summary, err := h.getProfileSummary.Execute(ctx, customerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.LogEvent("profile-summary.not-found", shared.LogFields{"customerId": customerID})
			shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "customerId": customerID})
			return
		}
		shared.LogEvent("profile-summary.error", shared.LogFields{"customerId": customerID, "message": "database_unavailable"})
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.LogEvent("profile-summary.found", shared.LogFields{
		"customerId":  summary.CustomerID,
		"loyaltyTier": summary.LoyaltyTier,
		"stage":       summary.Stage,
		"updatedAt":   summary.UpdatedAt.Format(time.RFC3339),
	})
	shared.CoreBusinessTransactionsTotal.WithLabelValues("profile_summary", "served").Inc()

	shared.WriteJSON(w, http.StatusOK, summary)
}
