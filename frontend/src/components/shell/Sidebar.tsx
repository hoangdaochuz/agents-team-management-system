import { NavLink, useNavigate } from "react-router-dom";
import { cn } from "../../lib/cn";
import { Icon, type IconName } from "../../lib/icons";
import { useUI } from "../../store/ui";
import { useAuth } from "../../store/auth";
import { auth } from "../../api/client";

interface NavItem {
  to: string;
  label: string;
  icon: IconName;
  end?: boolean;
  /** Workspace role required to see this entry. */
  needsRole?: "admin";
  /** When true, only platform superadmins see this entry. */
  needsSuperadmin?: boolean;
}

const NAV: { group: string; items: NavItem[] }[] = [
  {
    group: "Workspace",
    items: [
      { to: "/dashboard", label: "Dashboard", icon: "grid" },
      { to: "/board", label: "Kanban board", icon: "board" },
      { to: "/agents", label: "Agents", icon: "bot" },
      { to: "/agents/builder", label: "Agent builder", icon: "sparkle" },
      { to: "/workspaces", label: "Workspaces", icon: "building" },
    ],
  },
  {
    group: "Operations",
    items: [
      { to: "/history", label: "Run history", icon: "clock" },
      { to: "/settings", label: "Settings", icon: "gear" },
    ],
  },
  {
    group: "Administration",
    items: [
      { to: "/admin", label: "Members & roles", icon: "users", needsRole: "admin" },
      { to: "/sysadmin", label: "System console", icon: "shield", needsSuperadmin: true },
    ],
  },
];

export function Sidebar() {
  const open = useUI((s) => s.sidebarOpen);
  const hasRole = useAuth((s) => s.hasRole);
  const isSuperadmin = useAuth((s) => Boolean(s.user?.is_superadmin));
  const user = useAuth((s) => s.user);
  const activeWorkspace = useAuth((s) => s.activeWorkspace);
  const logout = useAuth((s) => s.logout);
  const navigate = useNavigate();

  const initials = (user?.name ?? "?")
    .split(" ")
    .map((p) => p[0])
    .slice(0, 2)
    .join("")
    .toUpperCase();

  const signOut = () => {
    // Invalidate the server-side session, then clear local state.
    auth.logout().catch(() => undefined).finally(() => {
      logout();
      navigate("/login", { replace: true });
    });
  };

  return (
    <aside className={cn("sidebar", open && "open")}>
      <div className="brand">
        <div className="brand-mark">◆</div>
        <div>
          <div className="brand-name">Agent Ops</div>
          <div className="brand-sub">{activeWorkspace?.name ?? "Control room"}</div>
        </div>
      </div>

      {NAV.map((g) => {
        const items = g.items.filter(
          (it) => (!it.needsRole || hasRole(it.needsRole)) && (!it.needsSuperadmin || isSuperadmin),
        );
        if (items.length === 0) return null;
        return (
          <div key={g.group}>
            <div className="nav-group-label">{g.group}</div>
            <nav className="nav">
              {items.map((it) => (
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
        );
      })}

      <div className="sidebar-foot">
        <div className="avatar">{initials}</div>
        <div style={{ flex: 1, minWidth: 0 }}>
          <div className="who-name">{user?.name ?? "Operator"}</div>
          <div className="who-role">
            <span className={`role-pill role-${activeWorkspace?.role ?? "member"}`}>
              {activeWorkspace?.role ?? "member"}
            </span>
          </div>
        </div>
        <button
          className="icon-btn"
          aria-label="Sign out"
          title="Sign out"
          onClick={signOut}
          style={{ width: 32, height: 32 }}
        >
          <Icon name="logout" size={16} />
        </button>
      </div>
    </aside>
  );
}
