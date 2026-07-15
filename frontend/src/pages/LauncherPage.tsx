import { useNavigate } from "react-router-dom";
import { Icon } from "../lib/icons";
import type { IconName } from "../lib/icons";

/** A screen card on the launcher hub. */
interface ScreenDef {
  to: string;
  icon: IconName;
  title: string;
  description: string;
}

const SCREENS: ScreenDef[] = [
  {
    to: "/dashboard",
    icon: "grid",
    title: "Dashboard",
    description: "KPI overview, running agents, the review queue, and recent activity.",
  },
  {
    to: "/board",
    icon: "board",
    title: "Kanban board",
    description: "Drag cards across Backlog, In progress, In review, Done — each task owned by one agent.",
  },
  {
    to: "/tasks/demo",
    icon: "code",
    title: "Task detail",
    description: "Live output streaming plus both feedback flows: pause mid-run and review after completion.",
  },
  {
    to: "/agents",
    icon: "bot",
    title: "Agents",
    description: "A roster of 5 agents with role, status, current task, and load.",
  },
  {
    to: "/history",
    icon: "clock",
    title: "Run history",
    description: "Every run, including revisions made from feedback — filter by agent or status.",
  },
  {
    to: "/settings",
    icon: "gear",
    title: "Settings",
    description: "Workspace, agent defaults, feedback & pausing rules, notifications, integrations.",
  },
];

export function LauncherPage() {
  const navigate = useNavigate();

  return (
    <div className="launcher">
      <div className="hero-block">
        <div className="hero-eyebrow">◆ AI Agent Operations</div>
        <h1 className="hero-title">Orchestrate your team of AI agents</h1>
        <p className="hero-sub">
          A kanban board assigns tasks to individual agents, you monitor output in real time, and you can
          step in — pause mid-run to correct, or review after completion so the agent can self-fix.
        </p>
        <div className="row" style={{ justifyContent: "center", marginTop: "var(--space-6)" }}>
          <button className="btn btn-primary" onClick={() => navigate("/dashboard")}>
            Open dashboard
          </button>
          <button className="btn btn-ghost" onClick={() => navigate("/board")}>
            View board
          </button>
        </div>
      </div>

      <div className="screen-grid">
        {SCREENS.map((s) => (
          <div
            key={s.to}
            className="screen-card"
            role="link"
            tabIndex={0}
            onClick={() => navigate(s.to)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                navigate(s.to);
              }
            }}
          >
            <div className="screen-thumb">
              <Icon name={s.icon} size={28} />
            </div>
            <h3>{s.title}</h3>
            <p>{s.description}</p>
          </div>
        ))}
      </div>

      <p className="muted center" style={{ fontSize: 13, marginTop: "var(--space-12)" }}>
        High-fidelity prototype · Apple visual system (neutral base + single blue accent). Every interaction
        works — tabs, kanban drag, feedback modals, terminal streaming.
      </p>
    </div>
  );
}
