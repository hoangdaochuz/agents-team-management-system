import { cn } from "../../lib/cn";

export function Progress({
  value,
  className,
}: {
  value: number; // 0..100
  className?: string;
}) {
  const v = Math.max(0, Math.min(100, value));
  return (
    <div className={cn("progress", className)} role="progressbar" aria-valuenow={v}>
      <i style={{ width: `${v}%` }} />
    </div>
  );
}
