import { useState, type ReactNode } from "react";
import { Badge, type BadgeTone } from "../ui/Badge";
import { Icon, type IconName } from "../../lib/icons";

export interface ResourceItem {
  id: string;
}

/**
 * Shared list for the workspace-resources tabs (Knowledge / Skills / Plugins /
 * MCP / Rules). Mirrors the prototype `.res-card` / `.res-item` structure:
 * icon · title+meta · status badge · trailing action.
 */
export function ResourceList<T extends ResourceItem>({
  items,
  icon,
  title,
  meta,
  status,
  trailing,
  addLabel,
  onAdd,
  searchPlaceholder = "Search…",
}: {
  items: T[];
  icon: IconName;
  title: (item: T) => ReactNode;
  meta: (item: T) => ReactNode;
  status: (item: T) => { tone: BadgeTone; label: string; dot?: boolean };
  trailing?: (item: T) => ReactNode;
  addLabel: string;
  onAdd: () => void;
  searchPlaceholder?: string;
}) {
  const [q, setQ] = useState("");
  const filtered = items.filter((it) => String(title(it)).toLowerCase().includes(q.toLowerCase()));

  return (
    <div>
      <div className="res-head">
        <div className="res-search" style={{ position: "relative", flex: 1, maxWidth: 320 }}>
          <span style={{ position: "absolute", left: 10, top: 8, color: "var(--meta)", display: "inline-grid", placeItems: "center" }}>
            <Icon name="search" size={16} />
          </span>
          <input
            className="input"
            placeholder={searchPlaceholder}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ paddingLeft: 34 }}
          />
        </div>
        <button type="button" className="btn btn-soft btn-sm" onClick={onAdd}>
          {addLabel}
        </button>
      </div>

      {filtered.length === 0 ? (
        <div className="muted" style={{ padding: "var(--space-6)" }}>
          {q ? "No matches." : "Nothing here yet."}
        </div>
      ) : (
        <div className="res-card card flush">
          {filtered.map((it) => {
            const st = status(it);
            return (
              <div className="res-item" key={it.id}>
                <span className="res-ico">
                  <Icon name={icon} size={18} />
                </span>
                <div style={{ flex: 1, minWidth: 0 }}>
                  <div className="res-title">{title(it)}</div>
                  <div className="res-meta">{meta(it)}</div>
                </div>
                <Badge tone={st.tone} dot={st.dot}>
                  {st.label}
                </Badge>
                {trailing && <span className="res-actions">{trailing(it)}</span>}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
