package httputil

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Decode reads JSON into v; on error it writes 400 and returns true (caller
// should return immediately).
func Decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return true
	}
	return false
}

// Respond writes out at status, or maps any of notFoundErrs to 404, else 500.
func Respond(w http.ResponseWriter, log *slog.Logger, where string, status int, out any, err error, notFoundErrs ...error) {
	if err == nil {
		WriteJSON(w, status, out)
		return
	}
	for _, nf := range notFoundErrs {
		if errors.Is(err, nf) {
			Error(w, http.StatusNotFound, "not found")
			return
		}
	}
	ServerError(w, log, where, err)
}

// RespondCreated writes out at 201 (otherwise behaves like Respond).
func RespondCreated(w http.ResponseWriter, log *slog.Logger, where string, out any, err error, notFoundErrs ...error) {
	Respond(w, log, where, http.StatusCreated, out, err, notFoundErrs...)
}

// RespondOK writes out at 200 (otherwise behaves like Respond).
func RespondOK(w http.ResponseWriter, log *slog.Logger, where string, out any, err error, notFoundErrs ...error) {
	Respond(w, log, where, http.StatusOK, out, err, notFoundErrs...)
}

// RespondDelete writes 204, or maps notFoundErrs to 404, else 500.
func RespondDelete(w http.ResponseWriter, log *slog.Logger, where string, err error, notFoundErrs ...error) {
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for _, nf := range notFoundErrs {
		if errors.Is(err, nf) {
			Error(w, http.StatusNotFound, "not found")
			return
		}
	}
	ServerError(w, log, where, err)
}
