import { Navigate, Route, Routes } from "react-router-dom";
import { AppLayout } from "./components/shell/AppLayout";
import { LauncherPage } from "./pages/LauncherPage";
import { DashboardPage } from "./pages/DashboardPage";
import { KanbanPage } from "./pages/KanbanPage";
import { TaskDetailPage } from "./pages/TaskDetailPage";
import { AgentsPage } from "./pages/AgentsPage";
import { HistoryPage } from "./pages/HistoryPage";
import { SettingsPage } from "./pages/SettingsPage";

export default function App() {
  return (
    <Routes>
      {/* Launcher is a standalone full-page hub (no app chrome). */}
      <Route path="/" element={<LauncherPage />} />

      {/* Everything else shares the sidebar + topbar shell. */}
      <Route element={<AppLayout />}>
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/board" element={<KanbanPage />} />
        <Route path="/tasks/:id" element={<TaskDetailPage />} />
        <Route path="/agents" element={<AgentsPage />} />
        <Route path="/history" element={<HistoryPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
