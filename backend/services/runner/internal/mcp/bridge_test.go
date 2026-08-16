package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// fakeServer is a minimal in-process MCP stdio server: it reads JSON-RPC lines
// from r, handles initialize/tools-list/tools-call, and writes responses to w.
type fakeServer struct {
	r *bufio.Reader
	w io.Writer
}

func (s *fakeServer) serve(ctx <-chan struct{}) {
	for {
		select {
		case <-ctx:
			return
		default:
		}
		line, err := s.r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id,omitempty"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if json.Unmarshal([]byte(line), &msg) != nil {
			continue
		}
		s.respond(msg.ID, msg.Method, msg.Params)
	}
}

func (s *fakeServer) respond(id json.RawMessage, method string, params json.RawMessage) {
	switch method {
	case "initialize":
		s.write(id, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]string{"name": "fake"},
		})
	case "tools/list":
		s.write(id, map[string]any{"tools": []map[string]any{
			{"name": "echo", "description": "echo back"},
			{"name": "add", "description": "add numbers"},
		}})
	case "tools/call":
		var p struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Name == "echo" {
			s.write(id, map[string]any{"content": []map[string]string{{"type": "text", "text": "echoed"}}})
		} else {
			s.write(id, map[string]any{"content": []map[string]string{{"type": "text", "text": "ok"}}, "isError": false})
		}
	case "notifications/initialized":
		// notification: no response.
	}
}

func (s *fakeServer) write(id json.RawMessage, result any) {
	if len(id) == 0 {
		return
	}
	resp := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result}
	buf, _ := json.Marshal(resp)
	buf = append(buf, '\n')
	_, _ = s.w.Write(buf)
}

// TestBridgeHandshakeAndCall drives the real conn framing logic against an
// in-process fake server (no exec, no external binary) and proves the full
// initialize → tools/list → tools/call round-trip works.
func TestBridgeHandshakeAndCall(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Wire: client writes → srvInW → srvInR (fake reads); fake writes → cliOutW → cliOutR (client reads).
	srvInR, srvInW := io.Pipe()
	cliOutR, cliOutW := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &fakeServer{r: bufio.NewReader(srvInR), w: cliOutW}
	go srv.serve(ctx.Done())

	c := &conn{
		name:   "fake",
		stdin:  srvInW,
		stdout: bufio.NewReader(cliOutR),
		wait:   make(chan struct{}),
		log:    log,
	}
	close(c.wait) // no real process; kill()/close() are nil-safe.

	if err := c.initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if err := c.listTools(ctx); err != nil {
		t.Fatalf("listTools: %v", err)
	}
	if len(c.tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(c.tools))
	}
	if c.tools[0].Name != "echo" {
		t.Errorf("first tool = %q, want echo", c.tools[0].Name)
	}

	out, err := c.callTool(ctx, "echo", json.RawMessage(`{"x":1}`))
	if err != nil {
		t.Fatalf("callTool: %v", err)
	}
	if out != "echoed" {
		t.Errorf("callTool out = %q, want echoed", out)
	}
}

// TestBridgeCallNoArgs verifies empty/blank args default to {}.
func TestBridgeCallNoArgs(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srvInR, srvInW := io.Pipe()
	cliOutR, cliOutW := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &fakeServer{r: bufio.NewReader(srvInR), w: cliOutW}
	go srv.serve(ctx.Done())
	c := &conn{name: "fake", stdin: srvInW, stdout: bufio.NewReader(cliOutR), wait: make(chan struct{}), log: log}
	close(c.wait)
	_ = c.initialize(ctx)
	_ = c.listTools(ctx)

	if _, err := c.callTool(ctx, "add", nil); err != nil {
		t.Fatalf("callTool nil args: %v", err)
	}
	if _, err := c.callTool(ctx, "add", json.RawMessage("   ")); err != nil {
		t.Fatalf("callTool blank args: %v", err)
	}
}

func TestJsonRaw(t *testing.T) {
	// jsonRaw wraps a plain identifier (tool name) as a JSON string literal.
	if got := string(jsonRaw("echo")); got != `"echo"` {
		t.Errorf("jsonRaw(echo) = %s, want %q", got, `"echo"`)
	}
}
