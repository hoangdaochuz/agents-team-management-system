package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// dockerAPIVersion is the Docker Engine API version targeted. The exec/start
// hijack path is stable across 1.4x.
const dockerAPIVersion = "v1.44"

// dockerEnv drives a credential-less sandbox container via the Docker Engine
// API over its unix socket. Only run_command (Exec) crosses into the container;
// file ops hit the host worktree (bind-mounted RW at /workspace).
type dockerEnv struct {
	cfg        Config
	wt         *Worktree
	log        *slog.Logger
	container  string
	httpClient *http.Client
}

func newDockerEnv(ctx context.Context, cfg Config, wt *Worktree, log *slog.Logger) (*dockerEnv, error) {
	hc := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", cfg.Socket)
			},
		},
		Timeout: 60 * time.Second,
	}
	e := &dockerEnv{cfg: cfg, wt: wt, log: log, httpClient: hc}
	if err := e.createAndStart(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *dockerEnv) WorktreeBranch() string { return e.wt.Branch }

// createAndStart creates the sandbox container with the worktree bind-mounted
// at /workspace and starts it. The image must already exist locally.
func (e *dockerEnv) createAndStart(ctx context.Context) error {
	body := map[string]any{
		"Image":      e.cfg.Image,
		"Cmd":        []string{"sleep", "infinity"},
		"WorkingDir": "/workspace",
		"HostConfig": map[string]any{
			"Binds":      []string{e.wt.Path + ":/workspace:rw"},
			"AutoRemove": false,
		},
	}
	var resp struct{ ID string `json:"Id"` }
	if err := e.doJSON(ctx, http.MethodPost, "/containers/create", body, &resp); err != nil {
		return fmt.Errorf("container create: %w", err)
	}
	e.container = resp.ID
	// Start is a no-body POST returning 204.
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.url("/containers/"+e.container+"/start"), nil)
	if resp, err := e.httpClient.Do(req); err != nil {
		return fmt.Errorf("container start: %w", err)
	} else {
_ = resp.Body.Close()
		if resp.StatusCode >= 300 {
			return fmt.Errorf("container start: %s", resp.Status)
		}
	}
	e.log.Info("sandbox container started", "container", shortID(e.container), "image", e.cfg.Image, "worktree", e.wt.Path)
	return nil
}

// Exec runs one command in the container via the exec API (Tty: true so the
// hijacked start stream is a single raw byte stream — combined stdout/stderr).
// The command runs against /workspace. ctx cancellation tears down the socket
// connection, aborting an in-flight command (task stop).
func (e *dockerEnv) Exec(ctx context.Context, cmd string, args ...string) (ExecResult, error) {
	create := map[string]any{
		"AttachStdout": true,
		"AttachStderr": true,
		"Tty":          true,
		"WorkingDir":   "/workspace",
		"Cmd":          append([]string{cmd}, args...),
	}
	var created struct{ ID string `json:"Id"` }
	if err := e.doJSON(ctx, http.MethodPost, "/containers/"+e.container+"/exec", create, &created); err != nil {
		return ExecResult{}, fmt.Errorf("exec create: %w", err)
	}
	out, err := e.execStart(ctx, created.ID)
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec start: %w", err)
	}
	code, err := e.execExitCode(ctx, created.ID)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{ExitCode: code, Stdout: string(out)}, nil
}

// execStart drives the hijacked exec/start endpoint over a raw unix-socket
// connection (net/http cannot express the Upgrade + raw-body stream cleanly).
// The daemon may answer either with a true 101 hijack (body streams until the
// process exits) or with a regular 200 + chunked body (Docker 29+); both are
// decoded via http.ReadResponse, which also surfaces non-2xx error bodies
// instead of leaving them stuck on the keep-alive connection.
func (e *dockerEnv) execStart(ctx context.Context, execID string) ([]byte, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", e.cfg.Socket)
	if err != nil {
		return nil, err
	}
defer func() { _ = conn.Close() }()
	// Cancel tear-down: closing the conn unblocks the read on stop.
	go func() { <-ctx.Done(); _ = conn.Close() }()

	payload := []byte(`{"Detach":false,"Tty":true}`)
	reqLine := "POST /" + dockerAPIVersion + "/exec/" + execID + "/start HTTP/1.1\r\n" +
		"Host: docker\r\nConnection: Upgrade\r\nContent-Type: application/json\r\n" +
		"Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n"
	if _, err := conn.Write(append([]byte(reqLine), payload...)); err != nil {
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodPost})
	if err != nil {
		return nil, err
	}
defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("exec start: %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return io.ReadAll(resp.Body)
}

// execExitCode fetches the exit code after exec start completes.
func (e *dockerEnv) execExitCode(ctx context.Context, execID string) (int, error) {
	var info struct {
		ExitCode int  `json:"ExitCode"`
		Running  bool `json:"Running"`
	}
	if err := e.doJSON(ctx, http.MethodGet, "/exec/"+execID+"/json", nil, &info); err != nil {
		return 0, err
	}
	return info.ExitCode, nil
}

// Logs returns the container's stdout+stderr output (task 7.4 secret-leak
// assertion; also useful for run diagnostics).
func (e *dockerEnv) Logs(ctx context.Context) (string, error) {
	resp, err := e.callRaw(ctx, http.MethodGet, "/containers/"+e.container+"/logs?stdout=1&stderr=1", nil)
	if err != nil {
		return "", err
	}
defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("docker logs: %s", resp.Status)
	}
	// The logs endpoint streams docker's 8-byte frame header per chunk; strip
	// them so the output is greppable text.
	var out strings.Builder
	br := bufio.NewReader(resp.Body)
	for {
		hdr := make([]byte, 8)
		if _, err := io.ReadFull(br, hdr); err != nil {
			break
		}
		n := int(hdr[4])<<24 | int(hdr[5])<<16 | int(hdr[6])<<8 | int(hdr[7])
		buf := make([]byte, n)
		if _, err := io.ReadFull(br, buf); err != nil {
			break
		}
		out.Write(buf)
	}
	return out.String(), nil
}

func (e *dockerEnv) ReadFile(_ context.Context, path string) (string, error) {
	return readHostFile(e.wt.Path, path)
}
func (e *dockerEnv) WriteFile(_ context.Context, path, content string) error {
	return writeHostFile(e.wt.Path, path, content)
}
func (e *dockerEnv) ListFiles(_ context.Context, path string) ([]string, error) {
	return listHostDir(e.wt.Path, path)
}

// Close stops + removes the container and removes the worktree.
func (e *dockerEnv) Close(ctx context.Context) error {
	var firstErr error
	if e.container != "" {
		// Best-effort stop (short timeout) then remove.
		_ = e.call(ctx, http.MethodPost, "/containers/"+e.container+"/stop?t=5", nil)
		if err := e.call(ctx, http.MethodDelete, "/containers/"+e.container+"?force=true", nil); err != nil {
			firstErr = err
		}
	}
	if err := removeWorktree(ctx, e.cfg.CloneRoot, filepath.Base(e.wt.Path)); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

// ── HTTP helpers ────────────────────────────────────────────────────────────

func (e *dockerEnv) url(path string) string { return "http://docker/" + dockerAPIVersion + path }

func (e *dockerEnv) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var r io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(buf)
	}
	resp, err := e.callRaw(ctx, method, path, r)
	if err != nil {
		return err
	}
defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("docker %s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(b)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (e *dockerEnv) call(ctx context.Context, method, path string, body any) error {
	return e.doJSON(ctx, method, path, body, nil)
}

func (e *dockerEnv) callRaw(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, e.url(path), body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return e.httpClient.Do(req)
}

func shortID(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// ── Host-side file ops (shared with the local fallback) ─────────────────────

func readHostFile(root, rel string) (string, error) {
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func writeHostFile(root, rel, content string) error {
	full := filepath.Join(root, rel)
	if dir := filepath.Dir(full); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

func listHostDir(root, rel string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, rel))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, en := range entries {
		name := en.Name()
		if !en.IsDir() {
			names = append(names, name)
			continue
		}
		names = append(names, name+"/")
	}
	return names, nil
}
