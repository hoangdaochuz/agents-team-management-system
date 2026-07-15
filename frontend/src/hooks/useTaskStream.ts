import { useEffect, useRef, useState } from "react";
import type { Step, StepKind } from "../api/types";

export type { Step, StepKind };

/**
 * Subscribe to the live agent step-log stream for a task over SSE.
 *
 * The backend emits events named "step" carrying the full `Step` shape
 * ({ id, run_id, seq, kind, payload, created_at }); it replays persisted steps
 * then tails new ones. EventSource handles reconnection natively, so a refresh
 * resumes the live view. Steps are deduped by `id` so a reconnect after replay
 * does not double-render.
 *
 * An "error" event sets `lastError` and does not clear accumulated steps.
 */
export function useTaskStream(taskId: string | null) {
  const [steps, setSteps] = useState<Step[]>([]);
  const [connected, setConnected] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);
  const seenRef = useRef<Set<string>>(new Set());
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!taskId) return;
    seenRef.current = new Set();
    const es = new EventSource(`/api/tasks/${taskId}/stream`);
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.addEventListener("step", (e) => {
      try {
        const step = JSON.parse((e as MessageEvent).data) as Step;
        if (seenRef.current.has(step.id)) return;
        seenRef.current.add(step.id);
        setSteps((prev) => [...prev, step]);
      } catch {
        // ignore malformed event
      }
    });
    es.addEventListener("error", (e) => {
      try {
        const data = JSON.parse((e as MessageEvent).data);
        setLastError(typeof data === "string" ? data : data?.message ?? "run error");
      } catch {
        setLastError("run error");
      }
    });

    return () => {
      es.close();
      esRef.current = null;
      setSteps([]);
      setConnected(false);
      setLastError(null);
    };
  }, [taskId]);

  return { steps, connected, lastError };
}
