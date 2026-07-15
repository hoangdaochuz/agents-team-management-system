import { cn } from "../../lib/cn";
import { hueFrom, initials } from "../../lib/format";

export function Avatar({
  name,
  text,
  size = "md",
  className,
}: {
  name?: string;
  text?: string;
  size?: "sm" | "md" | "lg";
  className?: string;
}) {
  const label = text ?? (name ? initials(name) : "??");
  const hue = hueFrom(name ?? label);
  return (
    <div
      className={cn("avatar", size !== "md" && `avatar ${size}`, className)}
      title={name ?? label}
      style={{
        backgroundImage: name
          ? `linear-gradient(135deg, hsl(${hue} 70% 55%), hsl(${(hue + 40) % 360} 70% 45%))`
          : undefined,
      }}
    >
      {label}
    </div>
  );
}

export function AvatarStack({
  names,
  max = 4,
  className,
}: {
  names: string[];
  max?: number;
  className?: string;
}) {
  const shown = names.slice(0, max);
  const extra = names.length - shown.length;
  return (
    <div className={cn("avatar-stack", className)}>
      {shown.map((n) => (
        <Avatar key={n} name={n} size="sm" />
      ))}
      {extra > 0 && (
        <span className="avatar more sm">+{extra}</span>
      )}
    </div>
  );
}
