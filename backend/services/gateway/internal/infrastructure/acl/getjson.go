// Package acl implements the Gateway's inter-service HTTP clients behind the
// application's focused ports (Anti-Corruption Layer): the Auth session
// resolver, the Orgs membership client, the Task ownership client, the Runner
// step replay client, and the stats fan-out client.
package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// StatusError marks a non-200 response from an internal endpoint. Callers use
// the code to distinguish definitive answers (404: the thing does not exist;
// 401/403: the token was rejected) from transient upstream trouble (5xx,
// timeouts), which must not be cached or reported as a client error.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("internal call returned %d %s", e.Code, e.Body)
}

// doGet performs a GET against url and decodes the JSON response into out.
// headers are added verbatim (the Auth client carries the session cookie and
// every client carries the shared internal token).
// Non-200 responses return a *StatusError; transport failures return the
// transport error (the caller degrades per error kind).
func doGet(hc *http.Client, log *slog.Logger, ctx context.Context, url string, headers http.Header, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		log.Warn("internal call failed", "url", url, "error", err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &StatusError{Code: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}
