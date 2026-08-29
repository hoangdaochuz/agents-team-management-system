// Package provision implements the orgs application's WorkspaceProvisioner
// port: a direct HTTP call to the Workspace service's internal provisioning
// endpoint, replacing the former workspace.created Kafka event.
package provision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts/events"
)

// Client provisions workspaces over the Workspace service's internal endpoint.
type Client struct {
	url string
	hc  *http.Client
}

// New builds the provisioner. An empty url makes it a no-op (returns nil so
// the caller can skip it entirely).
func New(url string) *Client {
	if url == "" {
		return nil
	}
	return &Client{url: strings.TrimSuffix(url, "/"), hc: &http.Client{Timeout: 5 * time.Second}}
}

// Provision implements application.WorkspaceProvisioner: POST
// /internal/workspaces/provision with the workspace-creation fact.
func (c *Client) Provision(ctx context.Context, d events.WorkspaceCreatedData) error {
	if c == nil || c.url == "" {
		return nil
	}
	body, err := json.Marshal(d)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url+"/internal/workspaces/provision", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("workspace provisioning returned %s", resp.Status)
	}
	return nil
}
