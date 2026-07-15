import type { Step } from "../../api/types";
import { Badge, Card, CardHead, EmptyState } from "../ui";
import { Icon } from "../../lib/icons";

function stepTitle(step: Step): { title: string; detail?: string } {
  if (step.kind === "message" || step.kind === "reasoning") {
    const content = "content" in step.payload ? step.payload.content : "";
    const first = content.split("\n")[0]?.trim() || step.kind;
    return { title: first.length > 70 ? `${first.slice(0, 67)}…` : first };
  }
  if (step.kind === "tool_call") {
    const tool = "tool" in step.payload ? step.payload.tool : "?";
    return { title: `Call ${tool}`, detail: "in progress" };
  }
  // tool_result
  const tool = "tool" in step.payload ? step.payload.tool : "?";
  return { title: `Run ${tool}`, detail: "done" };
}

export function StepsList({ steps }: { steps: Step[] }) {
  return (
    <Card>
      <CardHead title={<h2 className="card-title" style={{ fontSize: 16 }}>Steps</h2>} />
      {steps.length === 0 ? (
        <EmptyState icon="clock" title="No steps yet." hint="Steps appear as the agent runs." />
      ) : (
        <div className="stack" style={{ gap: "var(--space-3)" }}>
          {steps.map((step, i) => {
            const isLast = i === steps.length - 1;
            const running = isLast && step.kind === "tool_call";
            const done = step.kind === "tool_result" || !running;
            const { title, detail } = stepTitle(step);
            return (
              <div key={step.id} className="row" style={{ alignItems: "flex-start", gap: 10 }}>
                {done ? (
                  <span style={{ marginTop: 2 }}>
                    <Badge tone="success">
                      <Icon name="check" size={12} />
                    </Badge>
                  </span>
                ) : (
                  <span style={{ marginTop: 2 }}>
                    <Badge tone="accent" dot pulse>
                      {" "}
                    </Badge>
                  </span>
                )}
                <div style={{ opacity: done ? 1 : 0.9 }}>
                  <div style={{ fontSize: 14, fontWeight: 500 }}>{title}</div>
                  {detail && <div className="muted" style={{ fontSize: 12 }}>{detail}</div>}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </Card>
  );
}
