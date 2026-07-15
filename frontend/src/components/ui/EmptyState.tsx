import type { ReactNode } from "react";
import { Icon, type IconName } from "../../lib/icons";

export function EmptyState({
  icon = "search",
  title,
  hint,
  action,
}: {
  icon?: IconName;
  title: ReactNode;
  hint?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="center stack" style={{ padding: "var(--space-12) var(--space-6)", color: "var(--muted)" }}>
      <span style={{ opacity: 0.5 }}>
        <Icon name={icon} size={28} />
      </span>
      <div style={{ color: "var(--fg-2)", fontWeight: 600 }}>{title}</div>
      {hint && <div className="muted">{hint}</div>}
      {action}
    </div>
  );
}
