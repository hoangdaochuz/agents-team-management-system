import { useEffect, useRef, useState } from "react";

// Agent step shape (Phase 3+). Kept loose until the backend finalizes the schema.
export type AgentStep = {
  seq: number;
  kind: "message" | "tool_call" | "tool_result" | "reasoning";
  payload: unknown;
};

/**
 * Subscribe to the live agent step-log stream for a task over SSE.
 *
 * The backend replays persisted steps then tails new ones; EventSource handles
 * reconnection natively, so a refresh resumes the live view without dupes.
 */
export function useTaskStream(taskId: string | null) {
  const [steps, setSteps] = useState<AgentStep[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!taskId) return;
    const es = new EventSource(`/api/tasks/${taskId}/stream`);
    esRef.current = es;

    es.onopen = () => setConnected(true);
    es.onerror = () => setConnected(false);
    es.addEventListener("step", (e) => {
      try {
        const step = JSON.parse((e as MessageEvent).data) as AgentStep;
        setSteps((prev) => [...prev, step]);
      } catch {
        // ignore malformed event
      }
    });

    return () => {
      es.close();
      esRef.current = null;
      setSteps([]);
      setConnected(false);
    };
  }, [taskId]);

  return { steps, connected };
}
