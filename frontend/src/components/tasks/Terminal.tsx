import { useEffect, useRef } from "react";
import type { Step } from "../../api/types";

function describeToolArgs(args: unknown): string {
  if (args == null) return "";
  if (typeof args === "string") return args;
  try {
    const s = JSON.stringify(args);
    if (!s) return "";
    // truncate to keep a single terminal line readable
    return s.length > 80 ? `${s.slice(0, 77)}...` : s;
  } catch {
    return "";
  }
}

function isFinished(steps: Step[]): boolean {
  if (!steps.length) return false;
  const last = steps[steps.length - 1];
  return last.kind === "tool_result";
}

export function Terminal({ steps, connected }: { steps: Step[]; connected: boolean }) {
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [steps]);

  const empty = steps.length === 0;
  const finished = isFinished(steps);

  return (
    <div className="terminal" ref={ref}>
      {empty && (
        <span className="term-line term-muted">
          {connected ? "waiting for run…" : "waiting for run…"} <span className="caret" />
        </span>
      )}

      {steps.map((s) => {
        const ts = new Date(s.created_at).toLocaleTimeString(undefined, {
          hour: "2-digit",
          minute: "2-digit",
        });
        if (s.kind === "message" || s.kind === "reasoning") {
          const content = "content" in s.payload ? s.payload.content : "";
          return (
            <span key={s.id} className="term-line">
              <span className="term-prompt">{s.kind === "reasoning" ? "›" : "»"}</span>{" "}
              <span className="term-muted">[{ts}]</span> {content}
            </span>
          );
        }
        if (s.kind === "tool_call") {
          const p = s.payload as { tool: string; args: unknown };
          const argStr = describeToolArgs(p.args);
          return (
            <span key={s.id} className="term-line term-muted">
              → {p.tool}({argStr})
            </span>
          );
        }
        // tool_result
        const { tool } = "tool" in s.payload ? s.payload : { tool: "?" };
        return (
          <span key={s.id} className="term-line term-ok">
            ✓ {tool}
          </span>
        );
      })}

      {!empty && finished && (
        <span className="term-line term-ok">
          ✔ run completed — awaiting review
        </span>
      )}

      {!empty && !finished && connected && <span className="caret" />}
    </div>
  );
}
