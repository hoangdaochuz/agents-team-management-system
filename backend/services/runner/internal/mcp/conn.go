package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"

	"github.com/aaks/server/internal/contracts/resources"
)

// conn is one MCP stdio server connection. Requests are serialized per server
// (one in flight at a time) — sufficient for the agent loop's sequential tool
// dispatch and avoids response-reordering complexity.
type conn struct {
	name   string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	wait   chan struct{} // closed when the process exits
	mu     sync.Mutex
	nextID int
	tools  []toolInfo
	log    *slog.Logger
}

// dial launches the server process, runs the initialize handshake, and
// enumerates tools.
func dial(ctx context.Context, s resources.McpServer, log *slog.Logger) (*conn, error) {
	if s.Command == "" {
		return nil, fmt.Errorf("mcp server %q: empty command", s.Name)
	}
	c := exec.Command(s.Command, s.Args...)
	if len(s.Env) > 0 {
		c.Env = envSlice(s.Env)
	}
	stdin, err := c.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("start %q: %w", s.Command, err)
	}
	con := &conn{
		name: s.Name, cmd: c, stdin: stdin,
		stdout: bufio.NewReader(stdout), wait: make(chan struct{}), log: log,
	}
	go func() { _ = c.Wait(); close(con.wait) }()

	if err := con.initialize(ctx); err != nil {
		_ = con.close()
		return nil, err
	}
	if err := con.listTools(ctx); err != nil {
		_ = con.close()
		return nil, err
	}
	return con, nil
}

func envSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// initialize performs the MCP handshake.
func (c *conn) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    struct{}{},
		"clientInfo":      map[string]string{"name": "aaks-runner", "version": "1"},
	}
	if _, err := c.call(ctx, "initialize", params, 10*time.Second); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	// initialized notification (no id, no response expected).
	return c.notify("notifications/initialized", struct{}{})
}

// listTools enumerates the server's tools.
func (c *conn) listTools(ctx context.Context) error {
	raw, err := c.call(ctx, "tools/list", struct{}{}, 10*time.Second)
	if err != nil {
		return fmt.Errorf("tools/list: %w", err)
	}
	var out struct {
		Tools []toolInfo `json:"tools"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("tools/list decode: %w", err)
	}
	c.tools = out.Tools
	c.log.Info("mcp server connected", "server", c.name, "tools", len(c.tools))
	return nil
}

// callTool invokes a tool by name and returns its concatenated text output.
func (c *conn) callTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage("{}")
	}
	params := map[string]json.RawMessage{
		"name":      jsonRaw(name),
		"arguments": args,
	}
	raw, err := c.call(ctx, "tools/call", params, 60*time.Second)
	if err != nil {
		return "", err
	}
	var res callResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return string(raw), nil // tolerate non-conforming servers
	}
	text := ""
	for _, p := range res.Content {
		if p.Type == "text" || p.Type == "" {
			text += p.Text
		}
	}
	if res.IsError {
		return text, fmt.Errorf("mcp tool %q returned an error: %s", name, truncate(text, 200))
	}
	return text, nil
}

// call sends one JSON-RPC request and returns the matching result. It reads
// stdout line by line, skipping notifications until the response with the
// request's id arrives.
func (c *conn) call(ctx context.Context, method string, params any, timeout time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++
	id := c.nextID
	idBytes, _ := json.Marshal(id)

	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		req["params"] = params
	}
	buf, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	buf = append(buf, '\n')

	// Honour ctx + a per-call timeout for the write+read.
	done := make(chan struct{})
	defer close(done)
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	go func() {
		select {
		case <-ctx.Done():
			c.kill()
		case <-timer.C:
			c.kill()
		case <-done:
		}
	}()

	if _, err := c.stdin.Write(buf); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Result json.RawMessage `json:"result,omitempty"`
			Error  *rpcError       `json:"error,omitempty"`
		}
		if json.Unmarshal(line, &msg) != nil {
			continue // not JSON-RPC, skip
		}
		if len(msg.ID) == 0 {
			continue // notification
		}
		if !bytes.Equal(bytes.TrimSpace(msg.ID), bytes.TrimSpace(idBytes)) {
			continue // response to a different request
		}
		if msg.Error != nil {
			return nil, fmt.Errorf("rpc %d: %d %s", id, msg.Error.Code, msg.Error.Message)
		}
		return msg.Result, nil
	}
}

// notify sends a JSON-RPC notification (no id, no response).
func (c *conn) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	req := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		req["params"] = params
	}
	buf, err := json.Marshal(req)
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	_, err = c.stdin.Write(buf)
	return err
}

// kill stops the server process (best-effort, idempotent, nil-safe).
func (c *conn) kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

// close terminates the process and waits for it to exit.
func (c *conn) close() error {
	c.kill()
	_ = c.stdin.Close()
	select {
	case <-c.wait:
	case <-time.After(3 * time.Second):
	}
	return nil
}

func jsonRaw(s string) json.RawMessage { return json.RawMessage(`"` + s + `"`) }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
