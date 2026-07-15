import { cn } from "../../lib/cn";

export function Switch({
  checked,
  onChange,
  className,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  className?: string;
  label?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={cn("switch", checked && "on", className)}
      onClick={() => onChange(!checked)}
    />
  );
}
