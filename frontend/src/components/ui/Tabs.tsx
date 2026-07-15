import { useState, type ReactNode } from "react";
import { cn } from "../../lib/cn";

export interface TabDef {
  key: string;
  label: ReactNode;
  content: ReactNode;
}

/** Pill-style segmented tabs (`.tabs` / `.tab` / `.tab-panel`). */
export function Tabs({
  tabs,
  initial,
  onChange,
  className,
}: {
  tabs: TabDef[];
  initial?: string;
  onChange?: (key: string) => void;
  className?: string;
}) {
  const [active, setActive] = useState(initial ?? tabs[0]?.key);
  const current = tabs.find((t) => t.key === active) ?? tabs[0];
  return (
    <div className={className}>
      <div className="tabs">
        {tabs.map((t) => (
          <button
            key={t.key}
            className={cn("tab", t.key === active && "active")}
            onClick={() => {
              setActive(t.key);
              onChange?.(t.key);
            }}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="tab-panel active mt-6">{current?.content}</div>
    </div>
  );
}
