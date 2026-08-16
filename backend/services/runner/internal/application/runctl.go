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

// startRun launches a command run in a goroutine, keyed by task id. A second
// command for a task already in flight is skipped (log + no-op).
func (r *Runner) startRun(ctx context.Context, taskID string, fn func(context.Context)) {
	if _, inFlight := r.running.LoadOrStore(taskID, struct{}{}); inFlight {
		r.log.Info("run skipped: already in flight for task", "task_id", taskID)
		return
	}
	rctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancels.Store(taskID, cancel)
	go func() {
		defer func() {
			r.cancels.Delete(taskID)
			r.running.Delete(taskID)
			cancel()
		}()
		fn(rctx)
	}()
}

// CancelTask cancels any in-flight run for the task (stop command).
func (r *Runner) CancelTask(taskID string) {
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