import { Navigate, Route, Routes } from "react-router-dom";
import { AppGate } from "./components/auth/AppGate";
import { RequireAuth, RequireRole, RequireSuperadmin } from "./components/auth/RequireAuth";
import { AppLayout } from "./components/shell/AppLayout";
import { LauncherPage } from "./pages/LauncherPage";
import { DashboardPage } from "./pages/DashboardPage";
import { KanbanPage } from "./pages/KanbanPage";
import { TaskDetailPage } from "./pages/TaskDetailPage";
import { AgentsPage } from "./pages/AgentsPage";
import { HistoryPage } from "./pages/HistoryPage";
import { SettingsPage } from "./pages/SettingsPage";
import { LoginPage } from "./pages/LoginPage";
import { SignupPage } from "./pages/SignupPage";
import { PendingApprovalPage } from "./pages/PendingApprovalPage";
import { WorkspacesPage } from "./pages/WorkspacesPage";
import { WorkspaceResourcesPage } from "./pages/WorkspaceResourcesPage";
import { AgentBuilderPage } from "./pages/AgentBuilderPage";
import { AdminPage } from "./pages/AdminPage";
import { SysadminPage } from "./pages/SysadminPage";

export default function App() {
  return (
    <AppGate>
      <Routes>
        {/* Standalone auth flows (no app chrome). */}
        <Route path="/login" element={<LoginPage />} />
        <Route path="/signup" element={<SignupPage />} />
        <Route path="/pending" element={<PendingApprovalPage />} />

        {/* Launcher is a standalone full-page hub (still session-gated). */}
        <Route
          path="/"
          element={
            <RequireAuth>
              <LauncherPage />
            </RequireAuth>
          }
        />

        {/* Everything else: authenticated, sidebar + topbar shell. */}
        <Route
          element={
            <RequireAuth>
              <AppLayout />
            </RequireAuth>
          }
        >
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/board" element={<KanbanPage />} />
          <Route path="/tasks/:id" element={<TaskDetailPage />} />
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/agents/builder" element={<AgentBuilderPage />} />
          <Route path="/agents/builder/:id" element={<AgentBuilderPage />} />
          <Route path="/history" element={<HistoryPage />} />
          <Route path="/settings" element={<SettingsPage />} />
          <Route path="/workspaces" element={<WorkspacesPage />} />
          <Route path="/workspaces/:id/resources" element={<WorkspaceResourcesPage />} />
          <Route
            path="/admin"
            element={
              <RequireRole role="admin">
                <AdminPage />
              </RequireRole>
            }
          />
          <Route
            path="/sysadmin"
            element={
              <RequireSuperadmin>
                <SysadminPage />
              </RequireSuperadmin>
            }
          />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </AppGate>
  );
}
