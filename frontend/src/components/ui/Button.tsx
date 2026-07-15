import type { ButtonHTMLAttributes, ReactNode } from "react";
import { cn } from "../../lib/cn";

export type ButtonVariant =
  | "primary"
  | "dark"
  | "ghost"
  | "soft"
  | "danger";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: "sm" | "md";
  icon?: ReactNode;
}

const VARIANT: Record<ButtonVariant, string> = {
  primary: "btn-primary",
  dark: "btn-dark",
  ghost: "btn-ghost",
  soft: "btn-soft",
  danger: "btn-danger",
};

export function Button({
  variant = "primary",
  size = "md",
  icon,
  className,
  children,
  ...rest
}: ButtonProps) {
  return (
    <button
      className={cn("btn", VARIANT[variant], size === "sm" && "btn-sm", className)}
      {...rest}
    >
      {icon}
      {children}
    </button>
  );
}
