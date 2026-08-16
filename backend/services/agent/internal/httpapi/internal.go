package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/platform/http"
	"github.com/aaks/server/services/agent/internal/store"
)

// agentMcpServers returns an agent's attached MCP server *definitions*
// (hydrated from Catalog) so the Runner can bridge them as tools (task 5.5).
// The agent service owns the attachment; Catalog owns the definitions. This
// internal endpoint composes the two. Trusted callers only (no workspace gate).
func (a *App) agentMcpServers(w http.ResponseWriter, r *http.Request) {
	agent, err := a.store.GetUnscoped(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.RespondOK(w, a.log, "agent.McpServers", nil, err, store.ErrAgentNotFound)
		return
	}
	if len(agent.McpIDs) == 0 || a.catalogURL == "" {
		httputil.WriteJSON(w, http.StatusOK, []contracts.McpServer{})
		return
	}
	out, err := a.fetchMcpDefinitions(r.Context(), agent.McpIDs)
	if err != nil {
		// Degrade: return an empty list rather than failing the run setup.
		a.log.Warn("mcp definition hydration failed", "agent", agent.ID, "error", err)
		httputil.WriteJSON(w, http.StatusOK, []contracts.McpServer{})
		return
	}
	httputil.WriteJSON(w, http.StatusOK, out)
}

// fetchMcpDefinitions pulls the listed server definitions from Catalog's
// internal by-IDs endpoint.
func (a *App) fetchMcpDefinitions(ctx context.Context, ids []contracts.ID) ([]contracts.McpServer, error) {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = string(id)
	}
	url := strings.TrimSuffix(a.catalogURL, "/") + "/internal/mcp-servers?ids=" + strings.Join(parts, ",")

	req, cancel := GETCtx(ctx, url)
	defer cancel()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned %s", resp.Status)
	}
	var out []contracts.McpServer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// GETCtx builds a GET request with a 5s timeout context.
func GETCtx(ctx context.Context, url string) (*http.Request, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	return req, cancel
}
