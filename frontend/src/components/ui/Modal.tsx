import { useEffect, type ReactNode } from "react";
import { cn } from "../../lib/cn";
import { Icon } from "../../lib/icons";

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  className,
  wide = false,
}: {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  children: ReactNode;
  footer?: ReactNode;
  className?: string;
  wide?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  return (
    <div className={cn("overlay", open && "open")} onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className={cn("modal", wide && "modal-wide", className)} role="dialog" aria-modal="true">
        <div className="modal-head">
          <h3>{title}</h3>
          <button className="icon-btn" onClick={onClose} aria-label="Close" style={{ width: 32, height: 32 }}>
            <Icon name="close" size={16} />
          </button>
        </div>
        {children}
        {footer && <div className="modal-foot">{footer}</div>}
      </div>
    </div>
  );
}
