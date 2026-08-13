// Package httpapi registers the Agent-Runner routes: run/step/finding/artifact
// query endpoints (matching frontend/src/api/runs.ts) plus the command
// consumers (task.run-requested / task.review-requested / task.stop-requested /
// task.pr-open-requested) that drive runs and emit execution facts.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/IBM/sarama"

	"github.com/aaks/server/internal/contracts"
	"github.com/aaks/server/internal/httputil"
	"github.com/aaks/server/internal/kafka"
	"github.com/aaks/server/services/runner/internal/driver"
	"github.com/aaks/server/services/runner/internal/sandbox"
	"github.com/aaks/server/services/runner/internal/store"
)

// App holds the Runner service dependencies.
type App struct {
	store  *store.Store
	prod   sarama.SyncProducer
	driver driver.Driver
	log    *slog.Logger
	// settingsToken + settingsURL let the runner fetch provider keys from
	// Settings (in-memory only, per run). Empty = no keys available.
	settingsToken string
	settingsURL   string
	// resourcesURL lets the runner fetch the workspace's enabled rules
	// (task 12.3): enabled rules become per-workspace guardrails. Empty = no
	// rules enforced.
	resourcesURL string
	// agentURL lets the runner fetch the agent's attached MCP server definitions
	// (hydrated by the Agent service from Catalog) for task 5.5 bridging. Empty =
	// no MCP tools.
	agentURL string
	caps       driver.Caps
	// sandbox, when enabled (RUNNER_SANDBOX=docker|local), sets up a per-task
	// worktree + credential-less container for the LLM driver's built-in tools.
	sandbox *sandbox.Manager
	// cancels cancels in-flight runs per task (stop command); running guards
	// against concurrent runs for the same task.
	cancels sync.Map // taskID -> context.CancelFunc
	running sync.Map // taskID -> struct{}
}

// Register wires runner routes + the command consumers.
func Register(mux *http.ServeMux, log *slog.Logger) error {
	dsn := os.Getenv("RUNNER_DB_DSN")
	if dsn == "" {
		return errors.New("RUNNER_DB_DSN is not set")
	}
	st, err := store.New(context.Background(), dsn, log)
	if err != nil {
		return err
	}
	app := &App{
		store:         st,
		log:           log,
		driver:        driver.New(os.Getenv("RUNNER_DRIVER"), log),
		settingsToken: os.Getenv("SETTINGS_INTERNAL_TOKEN"),
		settingsURL:   os.Getenv("SETTINGS_URL"),
		resourcesURL: os.Getenv("RESOURCES_URL"),
		agentURL:     os.Getenv("AGENT_URL"),
		sandbox: sandbox.New(sandbox.Config{
			Kind:      os.Getenv("RUNNER_SANDBOX"),
			Image:     os.Getenv("RUNNER_SANDBOX_IMAGE"),
			Socket:    os.Getenv("RUNNER_DOCKER_SOCKET"),
			CloneRoot: os.Getenv("RUNNER_CLONE_ROOT"),
		}, log),
		caps: driver.Caps{
			MaxSteps:  envInt("RUNNER_MAX_STEPS", 50),
			MaxTokens: envInt("RUNNER_MAX_TOKENS", 100_000),
			WallClock: time.Duration(envInt("RUNNER_WALL_CLOCK_MIN", 30)) * time.Minute,
			StepDelay: 0,
		},
	}
	if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
		if p, err := kafka.NewProducer(kafka.Brokers(strings.Split(brokers, ",")), log); err != nil {
			log.Warn("kafka producer unavailable; runner emits no facts", "error", err)
		} else {
			app.prod = p
		}
	}

	// Query surface (served through the Gateway under /tasks/:id/* and /runs/:id/*).
	mux.HandleFunc("GET /tasks/{id}/runs", app.listRuns)
	mux.HandleFunc("GET /tasks/{id}/artifacts", app.listArtifacts)
	mux.HandleFunc("GET /runs/{id}/steps", app.listSteps)
	mux.HandleFunc("GET /runs/{id}/findings", app.listFindings)

	// Internal surface used only by the Gateway (SSE replay).
	mux.HandleFunc("GET /internal/tasks/{id}/steps", app.listTaskSteps)

	app.startConsumers()

	log.Info("runner routes registered", "endpoints", 4, "driver", os.Getenv("RUNNER_DRIVER"))
	return nil
}

// startConsumers subscribes to the runner command topics (best-effort).
func (a *App) startConsumers() {
	brokers := os.Getenv("KAFKA_BROKERS")
	if brokers == "" {
		return
	}
	bs := kafka.Brokers(strings.Split(brokers, ","))
	cg, err := kafka.NewConsumerGroup(bs, "runner-commands", a.log)
	if err != nil {
		a.log.Warn("runner consumers unavailable", "error", err)
		return
	}
	go func() {
		if err := cg.Run(context.Background(), []string{
			contracts.TopicTaskRunRequested, contracts.TopicTaskReviewRequested,
			contracts.TopicTaskStopRequested, contracts.TopicPrOpenRequested,
		}, a.consume); err != nil {
			a.log.Error("runner consumer stopped", "error", err)
		}
	}()
}

// ── Command consumers ───────────────────────────────────────────────────────

// consume dispatches runner commands. Runs execute asynchronously (a stop
// command must be consumable while a run is in flight); a per-task in-flight
// guard prevents overlapping runs, and stop cancels via the task context.
func (a *App) consume(ctx context.Context, env contracts.EventEnvelope) error {
	switch env.EventType {
	case contracts.TopicTaskRunRequested:
		var d contracts.RunRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		a.startRun(ctx, d.TaskID, func(rctx context.Context) { a.runImplementer(rctx, d) })
	case contracts.TopicTaskReviewRequested:
		var d contracts.ReviewRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		a.startRun(ctx, d.TaskID, func(rctx context.Context) { a.runReviewer(rctx, d) })
	case contracts.TopicTaskStopRequested:
		var d contracts.StopRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		a.cancelTask(d.TaskID)
	case contracts.TopicPrOpenRequested:
		var d contracts.PrOpenRequestedData
		if err := env.DecodeData(&d); err != nil {
			return err
		}
		return a.openPr(ctx, d)
	}
	return nil
}

// startRun launches a command run in a goroutine, keyed by task id.
func (a *App) startRun(ctx context.Context, taskID contracts.ID, fn func(context.Context)) {
	if _, inFlight := a.running.LoadOrStore(taskID, struct{}{}); inFlight {
		a.log.Info("run skipped: already in flight for task", "task_id", taskID)
		return
	}
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	a.cancels.Store(taskID, cancel)
	go func() {
		defer func() {
			a.cancels.Delete(taskID)
			a.running.Delete(taskID)
			cancel()
		}()
		fn(rctx)
	}()
}

// cancelTask cancels any in-flight run for the task (task 5.6 stop).
func (a *App) cancelTask(taskID contracts.ID) {
	if v, ok := a.cancels.Load(taskID); ok {
		if cancel, ok2 := v.(context.CancelFunc); ok2 {
			cancel()
			a.log.Info("stop requested; cancelling run", "task_id", taskID)
		}
	}
}

// runImplementer starts an implementer run, executes the driver, persists the
// result, and emits run.completed (+ finding.* + artifact for the run).
func (a *App) runImplementer(ctx context.Context, d contracts.RunRequestedData) {
	if d.AgentID == "" || d.TaskID == "" {
		a.log.Warn("ignoring run request without agent or task", "event", d)
		return
	}
	model := d.ModelOverride
	if model == "" {
		model = "default"
	}
	runID, err := a.store.CreateRun(ctx, d.TaskID, contracts.RunRoleImplementer, d.AgentID, model, d.RoundNo)
	if err != nil {
		a.log.Error("create implementer run failed", "error", err)
		return
	}
	a.publish(ctx, contracts.TopicRunStarted, map[string]any{"run_id": runID, "task_id": d.TaskID}, d.TaskID)

	rc := a.runContext(d.TaskID, runID, d.AgentID, contracts.RunRoleImplementer, d.RoundNo, model, d.Prompt, d.WorkspaceID)
	if rc.APIKey == "" && a.settingsURL != "" {
		a.log.Warn("no provider key fetched; run may fail", "run", runID)
	}
	tools, cleanup := a.setupToolsForRun(ctx, d.TaskID, d.Prompt, d.AgentID, d.WorkspaceID)
	if cleanup != nil {
		defer cleanup()
	}
	rc.Tools = tools
	a.executeAndFinish(ctx, d.TaskID, runID, rc)
}

// runReviewer starts a reviewer run over the implementer run and emits the
// verdict fact.
func (a *App) runReviewer(ctx context.Context, d contracts.ReviewRequestedData) {
	if d.AgentID == "" || d.RunID == "" {
		a.log.Warn("ignoring review request without agent or run", "event", d)
		return
	}
	imp, err := a.store.GetRun(ctx, d.RunID)
	if err != nil {
		a.log.Warn("review run skipped: implementer run not found", "run", d.RunID, "error", err)
		return
	}
	runID, err := a.store.CreateRun(ctx, d.TaskID, contracts.RunRoleReviewer, d.AgentID, imp.Model, d.RoundNo)
	if err != nil {
		a.log.Error("create reviewer run failed", "error", err)
		return
	}
	a.publish(ctx, contracts.TopicRunStarted, map[string]any{"run_id": runID, "task_id": d.TaskID}, d.TaskID)

	rc := a.runContext(d.TaskID, runID, d.AgentID, contracts.RunRoleReviewer, d.RoundNo, imp.Model, d.Prompt, d.WorkspaceID)
	tools, cleanup := a.setupToolsForRun(ctx, d.TaskID, d.Prompt, d.AgentID, d.WorkspaceID)
	if cleanup != nil {
		defer cleanup()
	}
	rc.Tools = tools
	res := a.executeAndFinish(ctx, d.TaskID, runID, rc)
	if res.Verdict != "" {
		a.publish(ctx, contracts.TopicVerdict, contracts.VerdictData{
			TaskID: d.TaskID, RunID: runID, RoundNo: d.RoundNo,
			Decision: res.Verdict, Summary: res.VerdictSummary,
		}, d.TaskID)
	}
}

// runContext assembles the driver context, fetching the provider key from
// Settings when configured (in-memory only, task 7.3) and the workspace's
// enabled rules from Resources (task 12.3).
func (a *App) runContext(taskID, runID, agentID contracts.ID, role contracts.RunRole, roundNo int, model, prompt string, workspaceID contracts.ID) driver.RunContext {
	rc := driver.RunContext{
		TaskID: taskID, RunID: runID, AgentID: agentID, Role: role, RoundNo: roundNo,
		Prompt: prompt, Model: model, Provider: "openai", Caps: a.caps, Log: a.log,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if a.settingsURL != "" && a.settingsToken != "" {
		if key, err := a.fetchKey(ctx, string(rc.Provider)); err != nil {
			a.log.Warn("no provider key from settings", "error", err)
		} else {
			rc.APIKey = key
		}
	}
	if a.resourcesURL != "" && workspaceID != "" {
		if rules, err := a.fetchRules(ctx, workspaceID); err != nil {
			a.log.Warn("no rules from resources", "workspace", workspaceID, "error", err)
		} else {
			rc.Rules = rules
		}
	}
	return rc
}

// executeAndFinish runs the driver, persists steps/artifacts, and emits the
// completion facts. Returns the result for the caller (verdict emission).
func (a *App) executeAndFinish(ctx context.Context, taskID, runID contracts.ID, rc driver.RunContext) driver.Result {
	sink := func(st contracts.Step) error {
		if _, err := a.store.AppendStep(ctx, runID, st.Seq, st.Kind, st.Payload); err != nil {
			return err
		}
		a.publish(ctx, contracts.TopicStep, contracts.StepData{Step: st}, taskID)
		return nil
	}
	res, err := a.driver.Execute(ctx, rc, sink)
	if err != nil {
		res.Status = contracts.RunAborted
		res.Error = err.Error()
	}
	// The run context may be cancelled by a stop command mid-run; the final
	// persist + run.completed must still go out or the saga never learns the
	// run ended (task would stay stuck in doing).
	finishCtx := context.WithoutCancel(ctx)
	for _, f := range res.Findings {
		f.RunID = runID
		if _, err := a.store.AddFinding(finishCtx, runID, f); err != nil {
			a.log.Warn("finding persist failed", "error", err)
		}
		fd := contracts.FindingData{Finding: f}
		fd.Finding.RunID = runID
		a.publish(finishCtx, contracts.TopicFinding, fd, taskID)
	}
	for _, ar := range res.Artifacts {
		run := runID
		if _, err := a.store.AddArtifact(finishCtx, taskID, &run, ar.Filename, ar.Kind, ar.Summary, ar.Additions, ar.Deletions); err != nil {
			a.log.Warn("artifact persist failed", "error", err)
		}
	}
	if err := a.store.FinishRun(finishCtx, runID, res.Status, res.TokenUsage, res.Error); err != nil {
		a.log.Error("finish run failed", "run", runID, "error", err)
	}
	a.publish(finishCtx, contracts.TopicRunCompleted, contracts.RunCompletedData{
		TaskID: taskID, RunID: runID, AgentID: rc.AgentID, Role: rc.Role,
		Status: res.Status, RoundNo: rc.RoundNo, TokenUsage: res.TokenUsage, Error: res.Error,
	}, taskID)
	return res
}

// fetchRules pulls the workspace's enabled rules from Resources (internal
// endpoint). Failures are non-fatal: a run proceeds without rule guardrails.
func (a *App) fetchRules(ctx context.Context, workspaceID contracts.ID) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(a.resourcesURL, "/")+"/internal/workspaces/"+string(workspaceID)+"/enabled-rules", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("resources returned %s", resp.Status)
	}
	var out []contracts.Rule
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

// fetchKey pulls a provider key from Settings (mTLS + shared token; the
// plaintext never leaves the process).
func (a *App) fetchKey(ctx context.Context, provider string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(a.settingsURL, "/")+"/internal/keys/"+provider, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Settings-Token", a.settingsToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
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

// openPr emits pr.opened for the task's latest run (git/gh execution is
// environment-dependent; the fact is emitted with a dev PR URL).
func (a *App) openPr(ctx context.Context, d contracts.PrOpenRequestedData) error {
	run, err := a.store.LatestRun(ctx, d.TaskID)
	if err != nil {
		a.log.Warn("open-pr skipped: no run for task", "task", d.TaskID, "error", err)
		return nil
	}
	base := os.Getenv("RUNNER_PR_BASE_URL")
	if base == "" {
		base = "https://github.com/example/repo"
	}
	a.publish(ctx, contracts.TopicPrOpened, contracts.PrOpenedData{
		TaskID: d.TaskID, RunID: run.ID, URL: base + "/pull/" + shortID(d.TaskID),
	}, d.TaskID)
	a.log.Info("pr opened (dev)", "task", d.TaskID, "run", run.ID)
	return nil
}

// ── Query handlers ──────────────────────────────────────────────────────────

func (a *App) listRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := a.store.ListRunsByTask(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.ServerError(w, a.log, "runner.ListRuns", err)
		return
	}
	if runs == nil {
		runs = []contracts.Run{}
	}
	httputil.WriteJSON(w, http.StatusOK, runs)
}

func (a *App) listArtifacts(w http.ResponseWriter, r *http.Request) {
	arts, err := a.store.ListArtifactsByTask(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.ServerError(w, a.log, "runner.ListArtifacts", err)
		return
	}
	if arts == nil {
		arts = []contracts.Artifact{}
	}
	httputil.WriteJSON(w, http.StatusOK, arts)
}

func (a *App) listSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := a.store.ListSteps(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.ServerError(w, a.log, "runner.ListSteps", err)
		return
	}
	if steps == nil {
		steps = []contracts.Step{}
	}
	httputil.WriteJSON(w, http.StatusOK, steps)
}

func (a *App) listFindings(w http.ResponseWriter, r *http.Request) {
	findings, err := a.store.ListFindings(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.ServerError(w, a.log, "runner.ListFindings", err)
		return
	}
	if findings == nil {
		findings = []contracts.Finding{}
	}
	httputil.WriteJSON(w, http.StatusOK, findings)
}

// listTaskSteps serves the SSE replay: all steps for a task, in run/seq order.
func (a *App) listTaskSteps(w http.ResponseWriter, r *http.Request) {
	steps, err := a.store.ListStepsByTask(r.Context(), r.PathValue("id"))
	if err != nil {
		httputil.ServerError(w, a.log, "runner.ListTaskSteps", err)
		return
	}
	if steps == nil {
		steps = []contracts.Step{}
	}
	httputil.WriteJSON(w, http.StatusOK, steps)
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// publish emits env to topic, keyed by taskID. No-op when the producer is nil.
func (a *App) publish(ctx context.Context, topic string, data any, taskID contracts.ID) {
	if a.prod == nil {
		return
	}
	env := contracts.EventEnvelope{TaskID: taskID, Data: data}
	if err := kafka.Publish(ctx, a.prod, topic, env, a.log); err != nil {
		a.log.Error("publish event failed", "topic", topic, "task_id", taskID, "error", err)
	}
}

func envInt(k string, def int) int {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func shortID(id contracts.ID) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
