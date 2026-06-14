package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/PabloJTechDev/loyalty-core-points/internal/shared"
)

// AuthHandler handles HTTP requests for the auth journey (logins + password changes).
type AuthHandler struct {
	listLogins         *ListLoginsUseCase
	getLogin           *GetLoginUseCase
	createLogin        *CreateLoginUseCase
	listPwdChanges     *ListPasswordChangesUseCase
	getPwdChange       *GetPasswordChangeUseCase
	createPwdChange    *CreatePasswordChangeUseCase
}

// NewAuthHandler constructs an AuthHandler.
func NewAuthHandler(
	listLogins *ListLoginsUseCase,
	getLogin *GetLoginUseCase,
	createLogin *CreateLoginUseCase,
	listPwdChanges *ListPasswordChangesUseCase,
	getPwdChange *GetPasswordChangeUseCase,
	createPwdChange *CreatePasswordChangeUseCase,
) *AuthHandler {
	return &AuthHandler{
		listLogins:      listLogins,
		getLogin:        getLogin,
		createLogin:     createLogin,
		listPwdChanges:  listPwdChanges,
		getPwdChange:    getPwdChange,
		createPwdChange: createPwdChange,
	}
}

// ─── Logins ──────────────────────────────────────────────────────────────────

// HandleLogins routes /v1/customer-logins (list/create).
func (h *AuthHandler) HandleLogins(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListLogins(w, r)
	case http.MethodPost:
		h.handleCreateLogin(w, r)
	default:
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
	}
}

// HandleGetLogin routes /v1/customer-logins/:loginId.
func (h *AuthHandler) HandleGetLogin(w http.ResponseWriter, r *http.Request) {
	loginID := strings.TrimPrefix(r.URL.Path, "/v1/customer-logins/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trace, err := h.getLogin.Execute(ctx, loginID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "loginId": loginID})
			return
		}
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, trace)
}

func (h *AuthHandler) handleListLogins(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := h.listLogins.Execute(ctx)
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, LoginListResponse{Total: len(items), Items: items})
}

func (h *AuthHandler) handleCreateLogin(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		LoginID           string `json:"loginId"`
		RequestID         string `json:"requestId"`
		TransactionID     string `json:"transactionId"`
		CustomerEmailHash string `json:"customerEmailHash"`
	}
	if err := shared.DecodeJSONBody(r, &payload); err != nil {
		shared.WriteBadRequest(w, err)
		return
	}

	if payload.LoginID == "" || payload.RequestID == "" || payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "loginId, requestId, transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	shared.LogEvent("login.received", shared.LogFields{
		"loginId":           payload.LoginID,
		"requestId":         payload.RequestID,
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	trace, err := h.createLogin.Execute(ctx, CreateLoginInput{
		LoginID:           payload.LoginID,
		RequestID:         payload.RequestID,
		TransactionID:     payload.TransactionID,
		CustomerEmailHash: payload.CustomerEmailHash,
	})
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.LogEvent("login.stored", shared.LogFields{
		"loginId":         trace.LoginID,
		"requestId":       trace.RequestID,
		"transactionId":   trace.TransactionID,
		"stage":           trace.Stage,
		"source":          trace.Source,
		"authenticatedAt": trace.AuthenticatedAt.Format(time.RFC3339),
	})
	shared.CoreBusinessTransactionsTotal.WithLabelValues("login", "accepted").Inc()

	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":          "accepted",
		"loginId":         trace.LoginID,
		"requestId":       trace.RequestID,
		"transactionId":   trace.TransactionID,
		"authenticatedAt": trace.AuthenticatedAt,
		"storage":         "postgres",
	})
}

// ─── Password Changes ─────────────────────────────────────────────────────────

// HandlePasswordChanges routes /v1/customer-password-changes (list/create).
func (h *AuthHandler) HandlePasswordChanges(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleListPasswordChanges(w, r)
	case http.MethodPost:
		h.handleCreatePasswordChange(w, r)
	default:
		shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "path": r.URL.Path})
	}
}

// HandleGetPasswordChange routes /v1/customer-password-changes/:requestId.
func (h *AuthHandler) HandleGetPasswordChange(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimPrefix(r.URL.Path, "/v1/customer-password-changes/")
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	trace, err := h.getPwdChange.Execute(ctx, requestID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			shared.WriteJSON(w, http.StatusNotFound, map[string]string{"status": "not_found", "requestId": requestID})
			return
		}
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, trace)
}

func (h *AuthHandler) handleListPasswordChanges(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := h.listPwdChanges.Execute(ctx)
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.WriteJSON(w, http.StatusOK, PasswordChangeListResponse{Total: len(items), Items: items})
}

func (h *AuthHandler) handleCreatePasswordChange(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RequestID         string `json:"requestId"`
		TransactionID     string `json:"transactionId"`
		CustomerEmailHash string `json:"customerEmailHash"`
	}
	if err := shared.DecodeJSONBody(r, &payload); err != nil {
		shared.WriteBadRequest(w, err)
		return
	}

	if payload.RequestID == "" || payload.TransactionID == "" || payload.CustomerEmailHash == "" {
		shared.WriteJSON(w, http.StatusBadRequest, map[string]string{
			"status":  "error",
			"message": "requestId, transactionId and customerEmailHash are required",
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	shared.LogEvent("password-change.received", shared.LogFields{
		"requestId":         payload.RequestID,
		"transactionId":     payload.TransactionID,
		"customerEmailHash": payload.CustomerEmailHash,
		"remote":            r.RemoteAddr,
	})

	trace, err := h.createPwdChange.Execute(ctx, CreatePasswordChangeInput{
		RequestID:         payload.RequestID,
		TransactionID:     payload.TransactionID,
		CustomerEmailHash: payload.CustomerEmailHash,
	})
	if err != nil {
		shared.WriteJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "message": "database_unavailable"})
		return
	}

	shared.LogEvent("password-change.stored", shared.LogFields{
		"requestId":     trace.RequestID,
		"transactionId": trace.TransactionID,
		"stage":         trace.Stage,
		"source":        trace.Source,
		"requestedAt":   trace.RequestedAt.Format(time.RFC3339),
	})
	shared.CoreBusinessTransactionsTotal.WithLabelValues("password_change", "accepted").Inc()

	shared.WriteJSON(w, http.StatusCreated, map[string]any{
		"status":        "accepted",
		"requestId":     trace.RequestID,
		"transactionId": trace.TransactionID,
		"requestedAt":   trace.RequestedAt,
		"storage":       "postgres",
	})
}
