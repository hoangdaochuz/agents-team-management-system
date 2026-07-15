import type { ReactNode } from "react";
import { cn } from "../../lib/cn";

export type BadgeTone =
  | "muted"
  | "success"
  | "warn"
  | "accent"
  | "danger";

const TONE: Record<BadgeTone, string> = {
  muted: "badge-muted",
  success: "badge-success",
  warn: "badge-warn",
  accent: "badge-accent",
  danger: "badge-danger",
};

export function Badge({
  tone = "muted",
  dot = false,
  pulse = false,
  className,
  children,
}: {
  tone?: BadgeTone;
  dot?: boolean;
  pulse?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <span className={cn("badge", TONE[tone], className)}>
      {dot && <span className={cn("badge-dot", pulse && "dot-running")} />}
      {children}
    </span>
  );
}

/** Status badge mapped from a run/task status string. */
const STATUS_TONE: Record<string, { tone: BadgeTone; label: string }> = {
  running: { tone: "accent", label: "Running" },
  doing: { tone: "accent", label: "In progress" },
  backlog: { tone: "muted", label: "Backlog" },
  review: { tone: "warn", label: "In review" },
  done: { tone: "success", label: "Done" },
  paused: { tone: "warn", label: "Paused" },
  idle: { tone: "muted", label: "Idle" },
  blocked: { tone: "danger", label: "Blocked" },
  aborted: { tone: "danger", label: "Aborted" },
  stopped: { tone: "muted", label: "Stopped" },
  cancelled: { tone: "muted", label: "Cancelled" },
  failed: { tone: "danger", label: "Failed" },
  revised: { tone: "warn", label: "Revised" },
};

export function StatusBadge({
  status,
  pulse,
}: {
  status: string;
  pulse?: boolean;
}) {
  const meta = STATUS_TONE[status] ?? { tone: "muted" as BadgeTone, label: status };
  const live = pulse || status === "running";
  return (
    <Badge tone={meta.tone} dot pulse={live}>
      {meta.label}
    </Badge>
  );
}
