package enrollment

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// EnrollmentHandler handles HTTP requests for the enrollment journey.
type EnrollmentHandler struct {
	list   *ListEnrollmentsUseCase
	get    *GetEnrollmentUseCase
	create *CreateEnrollmentUseCase
}

// NewEnrollmentHandler constructs an EnrollmentHandler.
func NewEnrollmentHandler(
	list *ListEnrollmentsUseCase,
	get *GetEnrollmentUseCase,
	create *CreateEnrollmentUseCase,
) *EnrollmentHandler {
	return &EnrollmentHandler{list: list, get: get, create: create}
}

// HandleEnrollments routes /v1/customer-enrollments (list/create).
func (h *EnrollmentHandler) HandleEnrollments(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
	}
}

// HandleGetEnrollment routes /v1/customer-enrollments/:transactionId (get by ID).
func (h *EnrollmentHandler) HandleGetEnrollment(w http.ResponseWriter, r *http.Request) {
	transactionID := strings.TrimPrefix(r.URL.Path, "/v1/customer-enrollments/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trace, err := h.get.Execute(ctx, transactionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "transactionId": transactionID})
			return
		}
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, trace)
}

func (h *EnrollmentHandler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := h.list.Execute(ctx)
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, EnrollmentListResponse{Total: len(items), Items: items})
}

func (h *EnrollmentHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		TransactionID     string `json:"transactionId"`
		CustomerEmailHash string `json:"customerEmailHash"`
	}
	if err := shared.DecodeJSONBody(r, &payload); err != nil {
		shared.WriteBadRequest(w, err)
		return
	}

	if payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	shared.LogEvent("enrollment.received", shared.LogFields{
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	trace, err := h.create.Execute(ctx, CreateEnrollmentInput{
		TransactionID:     payload.TransactionID,
		CustomerEmailHash: payload.CustomerEmailHash,
		RemoteAddr:        r.RemoteAddr,
	})
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.LogEvent("enrollment.stored", shared.LogFields{
		"transactionId": trace.TransactionID,
		"stage":         trace.Stage,
		"source":        trace.Source,
		"receivedAt":    trace.ReceivedAt.Format(time.RFC3339),
	})
	shared.CoreBusinessTransactionsTotal.WithLabelValues("enrollment", "accepted").Inc()

	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":        "accepted",
		"transactionId": trace.TransactionID,
		"receivedAt":    trace.ReceivedAt,
		"storage":       "postgres",
	})
}
