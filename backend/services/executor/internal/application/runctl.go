package application

import (
	"context"
	"sync"
)

// syncMap is a thin wrapper over sync.Map for the per-task run cancellation and
// in-flight registries (keyed by task id).
type syncMap struct{ m sync.Map }

func (s *syncMap) LoadOrStore(key any, val any) (actual any, loaded bool) {
	return s.m.LoadOrStore(key, val)
}

func (s *syncMap) Store(key, val any) { s.m.Store(key, val) }

func (s *syncMap) Delete(key any) { s.m.Delete(key) }

func (s *syncMap) Load(key any) (any, bool) { return s.m.Load(key) }

func (s *syncMap) LoadAndDelete(key any) (any, bool) { return s.m.LoadAndDelete(key) }

// startRun launches a command run in a goroutine, keyed by task id. A command
// arriving while a run for the task is in flight is queued (not ACKed-and-
// dropped): it is re-dispatched when the in-flight run finishes, preserving
// the saga's expectation that every command eventually executes. Only the
// latest command is kept — the saga is round-based, so a newer command
// supersedes an older queued one.
func (r *Runner) startRun(ctx context.Context, taskID string, fn func(context.Context)) {
	if _, inFlight := r.running.LoadOrStore(taskID, struct{}{}); inFlight {
		r.pending.Store(taskID, fn)
		r.log.Info("run deferred: already in flight for task (will re-dispatch on completion)",
			"task_id", taskID)
		return
	}
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancels.Store(taskID, cancel)
	go func() {
		defer func() {
			r.cancels.Delete(taskID)
			r.running.Delete(taskID)
			cancel()
			// Re-dispatch the command that arrived while this run was in
			// flight, if it was not cancelled away in the meantime.
			if v, ok := r.pending.LoadAndDelete(taskID); ok {
				if next, ok2 := v.(func(context.Context)); ok2 {
					r.startRun(context.Background(), taskID, next)
				}
			}
		}()
		fn(rctx)
	}()
}

// CancelTask cancels any in-flight run for the task (stop command) and drops
// any queued follow-up command — a stopped task must not spring back to life.
func (r *Runner) CancelTask(taskID string) {
	r.pending.Delete(taskID)
	if v, ok := r.cancels.Load(taskID); ok {
		if cancel, ok2 := v.(context.CancelFunc); ok2 {
			cancel()
			r.log.Info("stop requested; cancelling run", "task_id", taskID)
		}
	}
}

// publish emits a fact to the bus, keyed by taskID.
func (r *Runner) publish(ctx context.Context, topic string, data any, taskID string) {
	r.pub.Publish(ctx, topic, data, taskID)
}
