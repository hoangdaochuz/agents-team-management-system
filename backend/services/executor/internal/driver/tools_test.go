package driver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeExec is a recording driver.ToolExec for dispatchTool tests.
type fakeExec struct {
	runCmd  string
	runArgs []string
	fileOut string
}

func (f *fakeExec) Run(_ context.Context, cmd string, args ...string) (ToolExecResult, error) {
	f.runCmd, f.runArgs = cmd, args
	// dispatched as sh -c "<cmdline>"; args[1] is the command line.
	if cmd == "sh" && len(args) >= 2 && strings.Contains(args[1], "fail") {
		return ToolExecResult{ExitCode: 2, Stdout: "out", Stderr: "boom"}, nil
	}
	return ToolExecResult{ExitCode: 0, Stdout: "ran: " + strings.Join(args, " ")}, nil
}
func (f *fakeExec) ReadFile(_ context.Context, path string) (string, error) {
	return f.fileOut + ":" + path, nil
}
func (f *fakeExec) WriteFile(_ context.Context, path, content string) error {
	f.fileOut = content
	return nil
}
func (f *fakeExec) ListFiles(_ context.Context, path string) ([]string, error) {
	return []string{path + "/a", path + "/b"}, nil
}

func TestDispatchRunCommand(t *testing.T) {
	fx := &fakeExec{}
	rc := RunContext{Tools: ToolSet{Exec: fx}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "run_command", Args: "go test ./..."})
	if !strings.Contains(out, "ran:") {
		t.Errorf("run_command out = %q", out)
	}
	if fx.runCmd != "sh" || len(fx.runArgs) != 2 || fx.runArgs[0] != "-c" || fx.runArgs[1] != "go test ./..." {
		t.Errorf("run invoked as %q %v", fx.runCmd, fx.runArgs)
	}
}

func TestDispatchRunCommandNonZeroExit(t *testing.T) {
	rc := RunContext{Tools: ToolSet{Exec: &fakeExec{}}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "run_command", Args: "make fail"})
	if !strings.Contains(out, "[exit 2]") || !strings.Contains(out, "boom") {
		t.Errorf("non-zero exit out = %q", out)
	}
}

func TestDispatchReadFile(t *testing.T) {
	rc := RunContext{Tools: ToolSet{Exec: &fakeExec{fileOut: "BODY"}}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "read_file", Args: "src/main.go"})
	if out != "BODY:src/main.go" {
		t.Errorf("read_file out = %q", out)
	}
}

func TestDispatchReadFileJSONArg(t *testing.T) {
	rc := RunContext{Tools: ToolSet{Exec: &fakeExec{fileOut: "X"}}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "read_file", Args: `{"path":"a/b.go"}`})
	if out != "X:a/b.go" {
		t.Errorf("read_file json arg out = %q", out)
	}
}

func TestDispatchWriteFile(t *testing.T) {
	fx := &fakeExec{}
	rc := RunContext{Tools: ToolSet{Exec: fx}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "write_file", Args: `{"path":"f.txt","content":"hello"}`})
	if !strings.Contains(out, "ok: wrote f.txt") {
		t.Errorf("write_file out = %q", out)
	}
	if fx.fileOut != "hello" {
		t.Errorf("write_file content = %q", fx.fileOut)
	}
}

func TestDispatchWriteFileBadArgs(t *testing.T) {
	rc := RunContext{Tools: ToolSet{Exec: &fakeExec{}}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "write_file", Args: "garbage"})
	if !strings.Contains(out, "requires") {
		t.Errorf("bad write_file out = %q", out)
	}
}

func TestDispatchNoSandboxStubbed(t *testing.T) {
	out := dispatchTool(context.Background(), RunContext{}, toolCall{Name: "run_command", Args: "ls"})
	if !strings.Contains(out, "unavailable") {
		t.Errorf("stubbed out = %q", out)
	}
}

// fakeBridge is a recording driver.McpBridge.
type fakeBridge struct {
	tools  []McpTool
	called string
}

func (b *fakeBridge) Tools() []McpTool { return b.tools }
func (b *fakeBridge) Call(_ context.Context, server, name string, _ json.RawMessage) (string, error) {
	b.called = server + "." + name
	return "mcp-result", nil
}
func (b *fakeBridge) Close(context.Context) error { return nil }

func TestDispatchMcpNamespaced(t *testing.T) {
	b := &fakeBridge{tools: []McpTool{{Server: "fs", Name: "search"}}}
	rc := RunContext{Tools: ToolSet{MCP: b}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "fs.search", Args: `{}`})
	if out != "mcp-result" {
		t.Errorf("mcp out = %q", out)
	}
	if b.called != "fs.search" {
		t.Errorf("mcp called = %q", b.called)
	}
}

func TestDispatchMcpUnknownDelegates(t *testing.T) {
	b := &fakeBridge{tools: nil}
	rc := RunContext{Tools: ToolSet{MCP: b}}
	out := dispatchTool(context.Background(), rc, toolCall{Name: "weather", Args: `{}`})
	if out != "mcp-result" {
		t.Errorf("unknown mcp out = %q", out)
	}
}

func TestIsBuiltin(t *testing.T) {
	for _, n := range []string{"run_command", "run", "read_file", "write_file", "list_files"} {
		if !isBuiltin(n) {
			t.Errorf("isBuiltin(%q) = false", n)
		}
	}
	if isBuiltin("not_a_tool") {
		t.Error("isBuiltin(not_a_tool) = true")
	}
}

func TestParseWriteArgsSplit(t *testing.T) {
	path, content, ok := parseWriteArgs("path/to/f.go rest of content")
	if !ok || path != "path/to/f.go" || content != "rest of content" {
		t.Errorf("parseWriteArgs split = %q %q %v", path, content, ok)
	}
}

func TestParsePathArg(t *testing.T) {
	if got := parsePathArg(`  "a/b.go"  `); got != "a/b.go" {
		t.Errorf("parsePathArg quoted = %q", got)
	}
	if got := parsePathArg("plain/path"); got != "plain/path" {
		t.Errorf("parsePathArg plain = %q", got)
	}
}
