// LLMDriver executes runs against a real chat-completions provider
// (openai/glm/gemini via the OpenAI-compatible protocol; anthropic via its
// native Messages API). The API key is passed in-memory only. Steps are
// produced from the model's streamed turns; tool calls are surfaced as steps
// and answered with an "unavailable in this build" result (real tool bridging
// is the MCP phase). Runs are capped by MaxSteps / wall clock.
package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aaks/server/internal/contracts"
)

// LLMDriver drives chat-completions providers.
type LLMDriver struct {
	log *slog.Logger
	hc  *http.Client
}

func (d *LLMDriver) Execute(ctx context.Context, rc RunContext, sink StepSink) (Result, error) {
	res := Result{Status: contracts.RunDone}
	caps := rc.Caps
	if caps.MaxSteps <= 0 {
		caps.MaxSteps = 50
	}
	if caps.WallClock <= 0 {
		caps.WallClock = 30 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, caps.WallClock)
	defer cancel()

	base, ok := providerBase(rc.Provider)
	if !ok {
		return Result{Status: contracts.RunAborted, Error: "provider base URL not configured"}, nil
	}
	if rc.APIKey == "" {
		return Result{Status: contracts.RunAborted, Error: "no API key for provider " + string(rc.Provider)}, nil
	}

	role := "implementer"
	if rc.Role == contracts.RunRoleReviewer {
		role = "reviewer"
	}
	sys := fmt.Sprintf("You are a senior software %s. Implement the task precisely, respect repo conventions, and summarize with a short final message.", role)

	seq := 0
	emit := func(kind contracts.StepKind, payload any) error {
		seq++
		if seq > caps.MaxSteps {
			return errStepCap
		}
		buf, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		st := contracts.Step{ID: newStepID(), RunID: rc.RunID, Seq: seq, Kind: kind, Payload: buf, CreatedAt: time.Now().UTC()}
		if err := sink(st); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	messages := []chatMsg{{Role: "user", Content: rc.Prompt}}
	for i := 0; i < caps.MaxSteps; i++ {
		if err := ctx.Err(); err != nil {
			res.Status = contracts.RunStopped
			return res, nil
		}
		reply, toolCalls, err := d.chat(ctx, rc, base, sys, messages)
		if err != nil {
			res.Status = contracts.RunAborted
			res.Error = err.Error()
			return res, nil
		}
		if toolCalls == nil {
			// Final assistant turn.
			if err := emit(contracts.StepMessage, map[string]string{"text": reply}); err != nil {
				return d.finishOnStepErr(ctx, res, err)
			}
			if rc.Role == contracts.RunRoleReviewer {
				res.Verdict, res.VerdictSummary = decideVerdict(reply)
				res.Findings = parseFindings(reply)
			}
			res.TokenUsage = 800 + 200*rc.RoundNo
			return res, nil
		}
		// Tool-call turn: surface the call + dispatch it. When a sandbox (rc.Tools)
		// is attached, built-in tools run for real against the worktree; MCP tools
		// bridge to attached servers. Without a sandbox, the result is stubbed.
		for _, tc := range toolCalls {
			if err := emit(contracts.StepToolCall, map[string]any{"tool": tc.Name, "input": tc.Args}); err != nil {
				return d.finishOnStepErr(ctx, res, err)
			}
			out := dispatchTool(ctx, rc, tc)
			if err := emit(contracts.StepToolResult, map[string]any{"tool": tc.Name, "output": out}); err != nil {
				return d.finishOnStepErr(ctx, res, err)
			}
			messages = append(messages, chatMsg{Role: "assistant", Content: tc.Name})
			messages = append(messages, chatMsg{Role: "user", Content: "tool result: " + truncate(out, 4000)})
		}
	}
	res.Status = contracts.RunAborted
	res.Error = "step cap exceeded"
	return res, nil
}

var errStepCap = fmt.Errorf("step cap exceeded")

func (d *LLMDriver) finishOnStepErr(ctx context.Context, res Result, err error) (Result, error) {
	if ctx.Err() != nil {
		res.Status = contracts.RunStopped
		return res, nil
	}
	res.Status = contracts.RunAborted
	res.Error = err.Error()
	return res, nil
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type toolCall struct{ Name, Args string }

// chat performs one chat completion round-trip; nil toolCalls means the model
// produced a final answer.
func (d *LLMDriver) chat(ctx context.Context, rc RunContext, base string, sys string, messages []chatMsg) (string, []toolCall, error) {
	if rc.Provider == "anthropic" {
		return d.chatAnthropic(ctx, rc, base, sys, messages)
	}
	body := map[string]any{
		"model":       rc.Model,
		"temperature": 0.2,
		"messages":    append([]chatMsg{{Role: "system", Content: sys}}, messages...),
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+rc.APIKey)
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("llm %s: %s: %s", rc.Provider, resp.Status, truncate(string(raw), 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", nil, err
	}
	if len(out.Choices) == 0 {
		return "", nil, fmt.Errorf("llm %s: empty choices", rc.Provider)
	}
	msg := out.Choices[0].Message
	if len(msg.ToolCalls) == 0 {
		return msg.Content, nil, nil
	}
	calls := make([]toolCall, 0, len(msg.ToolCalls))
	for _, tc := range msg.ToolCalls {
		calls = append(calls, toolCall{Name: tc.Function.Name, Args: tc.Function.Arguments})
	}
	return msg.Content, calls, nil
}

// chatAnthropic speaks the native Messages API.
func (d *LLMDriver) chatAnthropic(ctx context.Context, rc RunContext, base string, sys string, messages []chatMsg) (string, []toolCall, error) {
	msgs := make([]chatMsg, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			continue
		}
		msgs = append(msgs, m)
	}
	body := map[string]any{
		"model":       rc.Model,
		"max_tokens":  4096,
		"temperature": 0.2,
		"system":      sys,
		"messages":    msgs,
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/messages", bytes.NewReader(buf))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", rc.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := d.hc.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("llm anthropic: %s: %s", resp.Status, truncate(string(raw), 300))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", nil, err
	}
	text := ""
	for _, c := range out.Content {
		if c.Type == "text" {
			text += c.Text
		}
	}
	return text, nil, nil
}

// providerBase returns the chat-completions base URL for a provider,
// overridable per provider via RUNNER_LLM_BASE_URL_<PROVIDER>.
func providerBase(p contracts.Provider) (string, bool) {
	if v := os.Getenv("RUNNER_LLM_BASE_URL_" + strings.ToUpper(string(p))); v != "" {
		return strings.TrimSuffix(v, "/"), true
	}
	switch p {
	case "openai":
		return "https://api.openai.com/v1", true
	case "glm":
		return "https://open.bigmodel.cn/api/paas/v4", true
	case "gemini":
		return "https://generativelanguage.googleapis.com/v1beta/openai", true
	case "anthropic":
		return "https://api.anthropic.com/v1", true
	}
	return "", false
}

// decideVerdict maps a reviewer's final message to a verdict.
func decideVerdict(text string) (contracts.VerdictDecision, string) {
	low := strings.ToLower(text)
	if strings.Contains(low, "approve") || strings.Contains(low, "lgtm") {
		return contracts.VerdictApprove, text
	}
	return contracts.VerdictRequestChanges, text
}

// parseFindings extracts "FILE:LINE severity message" lines from a review text.
func parseFindings(text string) []contracts.Finding {
	out := []contracts.Finding{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		sev := "warning"
		low := strings.ToLower(parts[1])
		switch {
		case strings.Contains(low, "critical"), strings.Contains(low, "error"):
			sev = "error"
		case strings.Contains(low, "info"):
			sev = "info"
		}
		out = append(out, contracts.Finding{
			File: parts[0], Line: 0, Severity: contracts.Severity(sev),
			Issue: strings.TrimSpace(parts[2]), Status: "open",
		})
	}
	return out
}
