package shared

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
)

const MaxBodyBytes = 1024 * 1024

var ErrPayloadTooLarge = errors.New("payload_too_large")

// WriteJSON writes a JSON-encoded payload with the given status code.
func WriteJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// WriteBadRequest writes a 400 Bad Request response.
func WriteBadRequest(w http.ResponseWriter, err error) {
	message := "invalid json payload"
	if errors.Is(err, ErrPayloadTooLarge) {
		message = "payload too large"
	}
	WriteJSON(w, http.StatusBadRequest, map[string]string{
		"status":  "error",
		"message": message,
	})
}

// DecodeJSONBody reads and decodes the request body into target.
func DecodeJSONBody(r *http.Request, target any) error {
	limitedBody := io.LimitReader(r.Body, MaxBodyBytes+1)
	body, err := io.ReadAll(limitedBody)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	if len(body) > MaxBodyBytes {
		return ErrPayloadTooLarge
	}

	if len(strings.TrimSpace(string(body))) == 0 {
		body = []byte("{}")
	}

	return json.Unmarshal(body, target)
}
