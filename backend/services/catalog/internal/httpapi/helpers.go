package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/aaks/server/internal/httputil"
)

// decode reads JSON into v; on error it writes 400 and returns true (caller returns).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := httputil.ReadJSON(r, v); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return true
	}
	return false
}

// respond writes out as 200, or maps notFoundErrs to 404, else 500.
func respond(w http.ResponseWriter, log *slog.Logger, where string, out any, err error, notFoundErrs ...error) {
	if err != nil {
		writeErr(w, log, where, err, notFoundErrs...)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// respondCreated writes out as 201, or maps notFoundErrs to 404, else 500.
func respondCreated(w http.ResponseWriter, log *slog.Logger, where string, out any, err error, notFoundErrs ...error) {
	if err != nil {
		writeErr(w, log, where, err, notFoundErrs...)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, out)
}

// respondDelete writes 204, or maps notFoundErrs to 404, else 500.
func respondDelete(w http.ResponseWriter, log *slog.Logger, where string, err error, notFoundErrs ...error) {
	if err != nil {
		writeErr(w, log, where, err, notFoundErrs...)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeErr(w http.ResponseWriter, log *slog.Logger, where string, err error, notFoundErrs ...error) {
	for _, nf := range notFoundErrs {
		if errors.Is(err, nf) {
			httputil.Error(w, http.StatusNotFound, "not found")
			return
		}
	}
	httputil.ServerError(w, log, where, err)
}
