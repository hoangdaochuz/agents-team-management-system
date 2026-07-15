import { NavLink } from "react-router-dom";
import { cn } from "../../lib/cn";
import { Icon, type IconName } from "../../lib/icons";
import { useUI } from "../../store/ui";

interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  end?: boolean;
}

const NAV: { group: string; items: NavItem[] }[] = [
  {
    group: "Workspace",
    items: [
      { to: "/dashboard", label: "Dashboard", icon: "grid" },
      { to: "/board", label: "Kanban board", icon: "board" },
      { to: "/agents", label: "Agents", icon: "bot" },
    ],
  },
  {
    group: "Operations",
    items: [
      { to: "/history", label: "Run history", icon: "clock" },
      { to: "/settings", label: "Settings", icon: "gear" },
    ],
  },
];

export function Sidebar() {
  const open = useUI((s) => s.sidebarOpen);
  return (
    <aside className={cn("sidebar", open && "open")}>
      <div className="brand">
        <div className="brand-mark">◆</div>
        <div>
          <div className="brand-name">Agent Ops</div>
          <div className="brand-sub">Control room</div>
        </div>
      </div>

      {NAV.map((g) => (
        <div key={g.group}>
          <div className="nav-group-label">{g.group}</div>
          <nav className="nav">
            {g.items.map((it) => (
              <NavLink
                key={it.to}
                to={it.to}
                end={it.end}
                onClick={() => useUI.getState().closeSidebar()}
                className={({ isActive }) => cn(isActive && "active")}
              >
                <span className="nav-ico">
                  <Icon name={it.icon} size={18} />
                </span>
                {it.label}
              </NavLink>
            ))}
          </nav>
        </div>
      ))}

      <div className="sidebar-foot">
        <div className="avatar">DA</div>
        <div>
          <div className="who-name">Dang Anh</div>
          <div className="who-role">Workspace owner</div>
        </div>
      </div>
    </aside>
  );
}
