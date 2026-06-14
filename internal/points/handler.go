package points

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// PointsHandler handles HTTP requests for the points journey.
type PointsHandler struct {
	accrue      *AccrueUseCase
	redeem      *RedeemUseCase
	getBalance  *GetBalanceUseCase
	getTxns     *GetTransactionsUseCase
	getStats    *GetStatsUseCase
}

// NewPointsHandler constructs a PointsHandler.
func NewPointsHandler(
	accrue *AccrueUseCase,
	redeem *RedeemUseCase,
	getBalance *GetBalanceUseCase,
	getTxns *GetTransactionsUseCase,
	getStats *GetStatsUseCase,
) *PointsHandler {
	return &PointsHandler{
		accrue:     accrue,
		redeem:     redeem,
		getBalance: getBalance,
		getTxns:    getTxns,
		getStats:   getStats,
	}
}

// HandleAccruePoints handles POST /v1/points/accrue
func (h *PointsHandler) HandleAccruePoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
		return
	}

	var req struct {
		CustomerID  string `json:"customerId"`
		Points      int    `json:"points"`
		ReferenceID string `json:"referenceId"`
		Source      string `json:"source"`
		Description string `json:"description"`
	}
	if err := shared.DecodeJSONBody(r, &req); err != nil {
		shared.WriteBadRequest(w, err)
		return
	}
	if req.CustomerID == "" || req.Points <= 0 || req.ReferenceID == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "message": "customerId, points > 0, and referenceId are required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.accrue.Execute(ctx, AccrueInput{
		CustomerID:  req.CustomerID,
		Points:      req.Points,
		ReferenceID: req.ReferenceID,
		Source:      req.Source,
		Description: req.Description,
	})
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_error"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"transactionId": result.TransactionID,
		"accrued":       result.Accrued,
	})
}

// HandleRedeemPoints handles POST /v1/points/redeem
func (h *PointsHandler) HandleRedeemPoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
		return
	}

	var req struct {
		CustomerID  string `json:"customerId"`
		Points      int    `json:"points"`
		ReferenceID string `json:"referenceId"`
		Source      string `json:"source"`
		Description string `json:"description"`
	}
	if err := shared.DecodeJSONBody(r, &req); err != nil {
		shared.WriteBadRequest(w, err)
		return
	}
	if req.CustomerID == "" || req.Points <= 0 || req.ReferenceID == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "message": "customerId, points > 0, and referenceId are required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	result, err := h.redeem.Execute(ctx, RedeemInput{
		CustomerID:  req.CustomerID,
		Points:      req.Points,
		ReferenceID: req.ReferenceID,
		Source:      req.Source,
		Description: req.Description,
	})
	if err != nil {
		errStr := err.Error()
		if strings.HasPrefix(errStr, "insufficient_points:") {
			// Parse available:requested from error message
			var available, requested int
			parts := strings.SplitN(errStr, ":", 3)
			if len(parts) == 3 {
				_, _ = parseInt(parts[1], &available)
				_, _ = parseInt(parts[2], &requested)
			}
			shared.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"status":    "insufficient_points",
				"available": available,
				"requested": requested,
			})
			return
		}
		if errStr == "customer_account_not_found" {
			shared.WriteJSON(w, http.StatusBadRequest, map[string]string{"status": "error", "message": "customer_account_not_found"})
			return
		}
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_error"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "ok",
		"transactionId":    result.TransactionID,
		"redeemed":         result.Redeemed,
		"remainingBalance": result.RemainingBalance,
	})
}

// HandlePointsRouter routes /v1/points/{customerId}/balance and /v1/points/{customerId}/transactions
func (h *PointsHandler) HandlePointsRouter(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/points/"), "/")
	if len(parts) < 2 {
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found"})
		return
	}
	customerID := parts[0]
	action := parts[1]

	switch action {
	case "balance":
		h.handleGetBalance(w, r, customerID)
	case "transactions":
		h.handleGetTransactions(w, r, customerID)
	default:
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found"})
	}
}

// HandleStats handles GET /v1/stats (points portion)
func (h *PointsHandler) HandleStats(w http.ResponseWriter, r *http.Request, statsExtra map[string]any) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	result, err := h.getStats.Execute(ctx)
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]any{
			"status":  "error",
			"service": "core-points",
			"message": "database_unavailable",
		})
		return
	}

	// Merge with extra stats passed from main (enrollment/auth/login counts)
	resp := map[string]any{
		"totalPointsInCirculation": result.TotalPointsInCirculation,
		"totalLifetimeAccrued":     result.TotalLifetimeAccrued,
		"totalLifetimeRedeemed":    result.TotalLifetimeRedeemed,
		"totalActiveAccounts":      result.TotalActiveAccounts,
	}
	for k, v := range statsExtra {
		resp[k] = v
	}

	shared.WriteJSON(w, http.StatusOK, resp)
}

func (h *PointsHandler) handleGetBalance(w http.ResponseWriter, r *http.Request, customerID string) {
	if r.Method != http.MethodGet {
		shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp, err := h.getBalance.Execute(ctx, customerID)
	if err != nil {
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "message": "account not found"})
		return
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}

func (h *PointsHandler) handleGetTransactions(w http.ResponseWriter, r *http.Request, customerID string) {
	if r.Method != http.MethodGet {
		shared.WriteJSON(w, http.StatusMethodNotAllowed, map[string]string{"status": "method_not_allowed"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	items, err := h.getTxns.Execute(ctx, customerID)
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}
	shared.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": len(items)})
}

func parseInt(s string, target *int) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	if target != nil {
		*target = n
	}
	return n, err
}
