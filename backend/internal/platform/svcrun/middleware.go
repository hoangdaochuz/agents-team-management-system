package svcrun

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
)

type ctxKey string

const reqIDKey ctxKey = "request_id"

// requestIDMiddleware ensures every request has an X-Request-Id (generating one
// if absent) and echoes it on the response.
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newReqID()
		}
		w.Header().Set("X-Request-Id", id)
		r = r.WithContext(context.WithValue(r.Context(), reqIDKey, id))
		next.ServeHTTP(w, r)
	})
}

// recoverMiddleware catches handler panics, logs them, and returns 500 without
// dropping the process.
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("handler panic",
					"request_id", r.Context().Value(reqIDKey),
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID retrieves the request id from context, or "" if unset.
func RequestID(ctx context.Context) string {
	v, _ := ctx.Value(reqIDKey).(string)
	return v
}

func newReqID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
