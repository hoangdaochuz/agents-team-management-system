import { useNavigate } from "react-router-dom";
import { Icon } from "../../lib/icons";
import { useUI } from "../../store/ui";

export function Topbar() {
  const navigate = useNavigate();
  const { toggleSidebar, search, setSearch } = useUI();

  return (
    <header className="topbar">
      <button
        className="burger"
        aria-label="Menu"
        onClick={toggleSidebar}
      >
        <Icon name="grid" size={18} />
      </button>

      <div className="search">
        <span style={{ color: "var(--meta)" }}>
          <Icon name="search" size={16} />
        </span>
        <input
          type="text"
          placeholder="Search tasks, agents, runs…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      <div className="topbar-actions">
        <button className="btn btn-ghost btn-sm" onClick={() => navigate("/board")}>
          <Icon name="plus" size={16} />
          <span>New task</span>
        </button>
        <button className="icon-btn" aria-label="Notifications">
          <Icon name="bell" size={18} />
          <span className="dot-badge" />
        </button>
      </div>
    </header>
  );
}
