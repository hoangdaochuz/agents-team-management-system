import type { ReactNode } from "react";
import { cn } from "../../lib/cn";

export function Card({
  flush = false,
  className,
  children,
}: {
  flush?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return <section className={cn("card", flush && "flush", className)}>{children}</section>;
}

export function CardHead({
  title,
  link,
  actions,
  className,
}: {
  title: ReactNode;
  link?: ReactNode;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("card-head", className)}>
      <div className="card-title">{title}</div>
      <div className="spread">
        {link && <span className="card-link">{link}</span>}
        {actions}
      </div>
    </div>
  );
}
