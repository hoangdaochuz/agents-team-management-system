// Package httputil provides small JSON helpers shared by all service handlers
// and the gateway, so response/error encoding is consistent everywhere.
package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// WriteJSON encodes v as JSON with the given status. It logs encoding errors but
// cannot do much about them once headers are written.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "error", err)
	}
}

// ReadJSON decodes the request body into v. It rejects malformed JSON with 400.
func ReadJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

// Error writes a JSON error object with the frontend's expected shape
// (`{ "error": "<message>" }`) and the given HTTP status. Internal details are
// NOT leaked — pass a user-safe message.
func Error(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// ServerError logs the internal err (with request context) and writes a generic
// 500 to the client without leaking details.
func ServerError(w http.ResponseWriter, log *slog.Logger, where string, err error) {
	log.Error("internal error", "where", where, "error", err)
	Error(w, http.StatusInternalServerError, "internal server error")
}

// IsClientError reports whether err is a known client-side decode/validation err
// (callers map these to 4xx rather than 5xx).
var ErrBadRequest = errors.New("bad request")
