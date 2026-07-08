import { useQuery } from "@tanstack/react-query";
import { fetchHealth } from "./api/client";

// Phase 0 scaffold shell. Real routes/kanban UI land in Phase 9.
export default function App() {
  const { data, isLoading, error } = useQuery({
    queryKey: ["health"],
    queryFn: fetchHealth,
  });

  return (
    <main className="app">
      <h1>AI Agent Kanban</h1>
      <p className="muted">MVP — scaffold. Backend status:</p>
      {isLoading && <p>checking…</p>}
      {error && <p className="error">backend unreachable ({String(error)})</p>}
      {data && <p className="ok">✓ {data.status}</p>}
    </main>
  );
}
