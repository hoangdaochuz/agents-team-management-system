// Package mcp implements a minimal MCP (Model Context Protocol) stdio client
// and a Bridge that aggregates an agent's attached MCP servers as a single tool
// surface for the Runner's agent loop (task 5.5).
//
// Each attached MCP server is launched as a child process speaking JSON-RPC 2.0
// over stdin/stdout (the MCP stdio transport). The client performs the
// `initialize` handshake, enumerates tools via `tools/list`, and exposes them
// (namespaced as `<server>.<tool>`) through driver.McpBridge. `tools/call`
// bridges a model tool invocation back to the owning server.
//
// The bridge is dependency-free (no external SDK) and degrades gracefully: a
// server that fails to start or handshake is skipped, and the run proceeds with
// whatever tools resolved.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/services/runner/internal/driver"
)

// Bridge aggregates attached MCP servers behind driver.McpBridge.
type Bridge struct {
	log   *slog.Logger
	conns []*conn
	tools []driver.McpTool
}

// New launches each server, performs the handshake, and enumerates tools.
// Servers that fail to start or handshake are skipped (non-fatal).
func New(ctx context.Context, servers []contracts.McpServer, log *slog.Logger) (*Bridge, error) {
	b := &Bridge{log: log}
	for _, s := range servers {
		c, err := dial(ctx, s, log)
		if err != nil {
			log.Warn("mcp server skipped", "server", s.Name, "error", err)
			continue
		}
		b.conns = append(b.conns, c)
		for _, t := range c.tools {
			b.tools = append(b.tools, driver.McpTool{
				Server:      s.Name,
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			})
		}
	}
	if len(b.conns) == 0 {
		return nil, fmt.Errorf("no mcp servers connected")
	}
	return b, nil
}

// Tools returns the namespaced tool surface.
func (b *Bridge) Tools() []driver.McpTool { return b.tools }

// Call invokes `<name>` on the server registered as `server`. If `server` is
// empty and exactly one connection exists, it is used.
func (b *Bridge) Call(ctx context.Context, server, name string, args json.RawMessage) (string, error) {
	c := b.find(server)
	if c == nil {
		return "", fmt.Errorf("mcp: no server %q for tool %q", server, name)
	}
	return c.callTool(ctx, name, args)
}

func (b *Bridge) find(server string) *conn {
	if server == "" && len(b.conns) == 1 {
		return b.conns[0]
	}
	for _, c := range b.conns {
		if c.name == server {
			return c
		}
	}
	return nil
}

// Close terminates every server process.
func (b *Bridge) Close(_ context.Context) error {
	var firstErr error
	for _, c := range b.conns {
		if err := c.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ── JSON-RPC tool shapes ────────────────────────────────────────────────────

type toolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type callResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// rpcError is a JSON-RPC 2.0 error object.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
