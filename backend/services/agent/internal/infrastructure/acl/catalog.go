// Package acl implements the Agent service's inter-service HTTP clients behind
// the application's focused ports (Anti-Corruption Layer).
package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
)

// CatalogClient hydrates MCP server definitions from the Catalog service's
// internal by-IDs endpoint.
type CatalogClient struct {
	url string
	hc  *http.Client
}

// NewCatalogClient builds the Catalog MCP client.
func NewCatalogClient(url string) *CatalogClient {
	return &CatalogClient{url: strings.TrimSuffix(url, "/"), hc: &http.Client{Timeout: 5 * time.Second}}
}

// FetchMcpServers pulls the listed server definitions from Catalog.
func (c *CatalogClient) FetchMcpServers(ctx context.Context, ids []identity.ID) ([]resources.McpServer, error) {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = string(id)
	}
	url := c.url + "/internal/mcp-servers?ids=" + strings.Join(parts, ",")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog returned %s", resp.Status)
	}
	var out []resources.McpServer
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}