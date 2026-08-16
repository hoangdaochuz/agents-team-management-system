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
)

// doGet performs a GET against url and decodes the JSON response into out.
// headers are added verbatim (the Auth client carries the session cookie).
// Non-200 responses and transport failures return an error (the caller
// degrades: 401 for identity, empty results for composition reads).
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
		return fmt.Errorf("internal call %s returned %s", url, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out)
}
