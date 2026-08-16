package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// dispatchTool resolves one model-emitted tool call against the run's ToolSet.
// Built-in tools (run_command/read_file/write_file/list_files) run against the
// attached sandbox; MCP server tools bridge through MCP. When no sandbox/MCP is
// attached the call is stubbed (dev/CI). The string return is the tool output
// surfaced to the model as a tool_result step.
func dispatchTool(ctx context.Context, rc RunContext, tc toolCall) string {
	name := strings.TrimSpace(tc.Name)
	args := strings.TrimSpace(tc.Args)

	// MCP bridge first: if a server owns this tool name (server.tool), or the
	// name is not a built-in and the bridge is present, delegate.
	if rc.Tools.MCP != nil {
		if isMcpTool(rc.Tools.MCP, name) {
			out, err := rc.Tools.MCP.Call(ctx, serverOf(name), toolOf(name), json.RawMessage(args))
			return toolOut(out, err)
		}
		if !isBuiltin(name) {
			// Unknown tool with an MCP bridge: try a best-effort call by bare name.
			out, err := rc.Tools.MCP.Call(ctx, "", name, json.RawMessage(args))
			return toolOut(out, err)
		}
	}

	if rc.Tools.Exec == nil || !isBuiltin(name) {
		return "tool unavailable in this build; state your next action"
	}
	switch name {
	case "run_command", "run":
		if rc.Tools.Exec == nil {
			return "no sandbox attached"
		}
		// Args is a shell command line; run via `sh -c` so pipes/quotes work.
		r, err := rc.Tools.Exec.Run(ctx, "sh", "-c", args)
		if err != nil {
			return fmt.Sprintf("run_command error: %v", err)
		}
		out := r.Stdout
		if r.Stderr != "" {
			out += "\n[stderr]\n" + r.Stderr
		}
		if r.ExitCode != 0 {
			out += fmt.Sprintf("\n[exit %d]", r.ExitCode)
		}
		return truncate(strings.TrimSpace(out), 8000)
	case "read_file":
		path := parsePathArg(args)
		s, err := rc.Tools.Exec.ReadFile(ctx, path)
		if err != nil {
			return fmt.Sprintf("read_file error: %v", err)
		}
		return truncate(s, 16000)
	case "list_files":
		path := parsePathArg(args)
		if path == "" {
			path = "."
		}
		names, err := rc.Tools.Exec.ListFiles(ctx, path)
		if err != nil {
			return fmt.Sprintf("list_files error: %v", err)
		}
		return strings.Join(names, "\n")
	case "write_file":
		path, content, ok := parseWriteArgs(args)
		if !ok {
			return "write_file requires {\"path\":..,\"content\":..}"
		}
		if err := rc.Tools.Exec.WriteFile(ctx, path, content); err != nil {
			return fmt.Sprintf("write_file error: %v", err)
		}
		return "ok: wrote " + path
	}
	return "unknown tool: " + name
}

func isBuiltin(name string) bool {
	switch name {
	case "run_command", "run", "read_file", "write_file", "list_files":
		return true
	}
	return false
}

func isMcpTool(b McpBridge, name string) bool {
	for _, t := range b.Tools() {
		if name == t.Server+"."+t.Name {
			return true
		}
	}
	return false
}

func serverOf(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[:i]
	}
	return ""
}

func toolOf(name string) string {
	if i := strings.IndexByte(name, '.'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func toolOut(out string, err error) string {
	if err != nil {
		return fmt.Sprintf("mcp error: %v", err)
	}
	return truncate(out, 16000)
}

// parsePathArg accepts a bare path, "path/to/x", or {"path":".."}.
func parsePathArg(args string) string {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "{") {
		var v struct {
			Path string `json:"path"`
			Cmd  string `json:"cmd"`
		}
		if json.Unmarshal([]byte(args), &v) == nil {
			if v.Path != "" {
				return v.Path
			}
			return v.Cmd
		}
	}
	return strings.Trim(args, "\"'")
}

// parseWriteArgs extracts path + content from {"path":..,"content":..}, falling
// back to "<path> <rest-as-content>".
func parseWriteArgs(args string) (string, string, bool) {
	args = strings.TrimSpace(args)
	if strings.HasPrefix(args, "{") {
		var v struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal([]byte(args), &v) == nil && v.Path != "" {
			return v.Path, v.Content, true
		}
	}
	if sp := strings.IndexByte(args, ' '); sp > 0 {
		return args[:sp], args[sp+1:], true
	}
	return "", "", false
}
