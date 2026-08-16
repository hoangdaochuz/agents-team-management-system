// Package acl implements the Runner's inter-service HTTP clients behind the
// application's focused ports (Anti-Corruption Layer, ISP: one client per
// upstream).
package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/identity"
	"github.com/aaks/server/internal/contracts/resources"
	"github.com/aaks/server/services/runner/internal/application"
)

// SettingsClient fetches provider keys from Settings (shared token; the
// plaintext key never leaves the process).
type SettingsClient struct {
	url   string
	token string
	hc    *http.Client
}

// NewSettingsClient builds the Settings key client. An empty url makes it a
// no-op returning application.ErrNotConfigured.
func NewSettingsClient(url, token string) *SettingsClient {
	return &SettingsClient{url: strings.TrimSuffix(url, "/"), token: token, hc: &http.Client{Timeout: 5 * time.Second}}
}

// FetchKey pulls a provider key from Settings.
func (c *SettingsClient) FetchKey(ctx context.Context, provider string) (string, error) {
	if c.url == "" {
		return "", application.ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/internal/keys/"+provider, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Settings-Token", c.token)
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("settings returned %s", resp.Status)
	}
	var out struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.APIKey, nil
}

// ResourcesClient fetches the workspace's enabled rules from Resources.
type ResourcesClient struct {
	url string
	hc  *http.Client
}

// NewResourcesClient builds the Resources rules client.
func NewResourcesClient(url string) *ResourcesClient {
	return &ResourcesClient{url: strings.TrimSuffix(url, "/"), hc: &http.Client{Timeout: 5 * time.Second}}
}

// FetchEnabledRules pulls the workspace's enabled rules (internal endpoint).
func (c *ResourcesClient) FetchEnabledRules(ctx context.Context, workspaceID identity.ID) ([]string, error) {
	if c.url == "" {
		return nil, application.ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.url+"/internal/workspaces/"+string(workspaceID)+"/enabled-rules", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resources returned %s", resp.Status)
	}
	var out []resources.Rule
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	rules := make([]string, 0, len(out))
	for _, r := range out {
		if r.Enabled {
			rules = append(rules, r.Name)
		}
	}
	return rules, nil
}

// AgentClient fetches the agent's attached MCP server definitions from the
// Agent service (hydrated from Catalog).
type AgentClient struct {
	url string
	hc  *http.Client
	log logAdapter
}

// Log is the minimal logger seam the client needs for its warn logs.
type logAdapter interface {
	Warn(msg string, args ...any)
}

// NewAgentClient builds the Agent MCP client.
func NewAgentClient(url string, log logAdapter) *AgentClient {
	return &AgentClient{url: strings.TrimSuffix(url, "/"), hc: &http.Client{Timeout: 5 * time.Second}, log: log}
}

// FetchMcpServers pulls the agent's attached MCP server definitions.
func (c *AgentClient) FetchMcpServers(ctx context.Context, agentID identity.ID) ([]resources.McpServer, error) {
	if c.url == "" || agentID == "" {
		return nil, application.ErrNotConfigured
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.url+"/internal/agents/"+string(agentID)+"/mcp-servers", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warn("mcp servers fetch failed", "agent", agentID, "error", err)
		}
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned %s", resp.Status)
	}
	var out []resources.McpServer
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}