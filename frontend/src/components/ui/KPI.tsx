import type { ReactNode } from "react";
import { cn } from "../../lib/cn";

export function KPI({
  label,
  value,
  delta,
  trend,
  spark,
  className,
}: {
  label: ReactNode;
  value: ReactNode;
  delta?: ReactNode;
  trend?: "up" | "down";
  spark?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("kpi", className)}>
      <div className="kpi-label">{label}</div>
      <div className="kpi-value">{value}</div>
      {delta && (
        <div className={cn("kpi-delta", trend === "up" && "up", trend === "down" && "down")}>{delta}</div>
      )}
      {spark && <div className="kpi-spark">{spark}</div>}
    </div>
  );
}

/** Minimal inline sparkline (no deps). */
export function Sparkline({
  points,
  width = 120,
  height = 28,
}: {
  points: number[];
  width?: number;
  height?: number;
}) {
  if (points.length < 2) return null;
  const min = Math.min(...points);
  const max = Math.max(...points);
  const span = max - min || 1;
  const step = width / (points.length - 1);
  const d = points
    .map((p, i) => `${i === 0 ? "M" : "L"}${(i * step).toFixed(1)},${(height - ((p - min) / span) * height).toFixed(1)}`)
    .join(" ");
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none">
      <path d={d} fill="none" stroke="var(--accent)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}
