import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { sysadmin } from "../api/client";
import type { AuditEntry, FeatureFlag, Organization, ServiceHealth, SignupRequest, SystemKpis } from "../api/types";
import { Badge } from "../components/ui/Badge";
import { KPI } from "../components/ui/KPI";
import { Switch } from "../components/ui/Switch";
import { useToast } from "../components/ui/Toast";

export function SysadminPage() {
  const { toast } = useToast();
  const kpisQ = useQuery({ queryKey: ["sysadmin", "kpis"], queryFn: () => sysadmin.kpis() });

  return (
    <>
      <div className="page-head">
        <div>
          <h1 className="page-title">
            System console <span className="sys-badge">SUPERADMIN</span>
          </h1>
          <div className="page-sub">Cross-organization administration, health, and feature flags</div>
        </div>
        <div className="row" style={{ gap: "var(--space-2)" }}>
          <button className="btn btn-ghost" onClick={() => toast("System audit view is read-only for now.", "bell")}>
            System audit
          </button>
          <button
            className="btn btn-primary"
            onClick={() =>
              sysadmin
                .runMaintenance()
                .then(() => toast("Maintenance job queued.", "check"))
                .catch(() => toast("Couldn't reach the backend.", "alert"))
            }
          >
            Run maintenance
          </button>
        </div>
      </div>

      <SysadminKpis data={kpisQ.data} loading={kpisQ.isLoading} />

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr", gap: "var(--space-5)", alignItems: "start" }}>
        <SystemHealthCard />
        <FeatureFlagsCard />
      </div>

      <SystemAuditFeed />
      <OrganizationsTable />
      <SignupRequestsTable />
    </>
  );
}

/* ── KPIs ───────────────────────────────────────────────────────── */
function SysadminKpis({ data, loading }: { data: SystemKpis | undefined; loading: boolean }) {
  if (loading || !data) {
    return (
      <div className="muted" style={{ padding: "var(--space-4) 0" }}>
        Loading system metrics… (endpoints aren't implemented yet.)
      </div>
    );
  }
  return (
    <div className="grid sa-kpis" style={{ marginBottom: "var(--space-5)" }}>
      <KPI label="Organizations" value={String(data.organizations)} delta={data.orgs_delta ? `+${data.orgs_delta} this month` : undefined} />
      <KPI label="Workspaces" value={String(data.workspaces)} delta={`across ${data.organizations} orgs`} />
      <KPI label="Active users (24h)" value={String(data.active_users_24h)} delta={data.active_users_delta ? `+${data.active_users_delta} today` : undefined} />
      <KPI label="Open seats" value={String(data.open_seats)} delta={data.open_seats_delta ? `${data.open_seats_delta} over plan` : undefined} />
    </div>
  );
}

/* ── System health ──────────────────────────────────────────────── */
function SystemHealthCard() {
  const { data, isLoading } = useQuery({ queryKey: ["sysadmin", "health"], queryFn: () => sysadmin.systemHealth() });
  const services: ServiceHealth[] = data?.services ?? [];
  return (
    <section className="card">
      <div className="card-head">
        <h2 className="card-title">System health</h2>
      </div>
      {(isLoading || services.length === 0) && (
        <div className="muted" style={{ padding: "var(--space-4) 0" }}>Loading health…</div>
      )}
      {services.map((s) => (
        <div className="health-row" key={s.name}>
          <span className="health-name">{s.name}</span>
          <span className="health-bar">
            <i
              style={{
                width: `${s.pct}%`,
                background: s.status === "ok" ? "var(--success)" : s.status === "warn" ? "var(--warn)" : "var(--danger)",
              }}
            />
          </span>
          <span className="health-val">{s.pct.toFixed(1)}%</span>
        </div>
      ))}
    </section>
  );
}

/* ── Feature flags ──────────────────────────────────────────────── */
function FeatureFlagsCard() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data } = useQuery({ queryKey: ["sysadmin", "flags"], queryFn: () => sysadmin.listFeatureFlags() });
  const toggle = useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean }) => sysadmin.toggleFeatureFlag(key, enabled),
    onMutate: ({ key, enabled }) => {
      const prev = qc.getQueryData<FeatureFlag[]>(["sysadmin", "flags"]);
      qc.setQueryData<FeatureFlag[]>(["sysadmin", "flags"], (l) =>
        (l ?? []).map((f) => (f.key === key ? { ...f, enabled } : f)),
      );
      return { prev };
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(["sysadmin", "flags"], ctx.prev);
      toast("Couldn't reach the backend.", "alert");
    },
    onSuccess: () => toast("Flag updated.", "check"),
  });
  const flags: FeatureFlag[] = data ?? [];
  return (
    <section className="card">
      <div className="card-head">
        <h2 className="card-title">Feature flags</h2>
      </div>
      {flags.length === 0 && <div className="muted" style={{ padding: "var(--space-4) 0" }}>Loading flags…</div>}
      {flags.map((f) => (
        <div className="flag-row" key={f.key}>
          <div>
            <div className="flag-name">{f.label}</div>
            {f.description && <div className="flag-desc">{f.description}</div>}
          </div>
          <Switch checked={f.enabled} onChange={(v) => toggle.mutate({ key: f.key, enabled: v })} label={f.label} />
        </div>
      ))}
    </section>
  );
}

/* ── System audit feed ──────────────────────────────────────────── */
function SystemAuditFeed() {
  const { data } = useQuery({ queryKey: ["sysadmin", "audit"], queryFn: () => sysadmin.systemAudit() });
  const entries: AuditEntry[] = data ?? [];
  if (entries.length === 0) return null;
  return (
    <section className="card" style={{ marginTop: "var(--space-5)" }}>
      <div className="card-head">
        <h2 className="card-title">System audit feed</h2>
      </div>
      <div className="feed">
        {entries.map((e) => (
          <div className="feed-item" key={e.id}>
            <span className="feed-dot">SY</span>
            <div style={{ flex: 1 }}>
              <div className="feed-text">
                <b>{e.actor.name}</b> {e.action}
                {e.target ? ` · ${e.target}` : ""}
              </div>
            </div>
            <span className="feed-time">{timeAgo(e.created_at)}</span>
          </div>
        ))}
      </div>
    </section>
  );
}

/* ── Organizations ──────────────────────────────────────────────── */
function OrganizationsTable() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data } = useQuery({ queryKey: ["sysadmin", "orgs"], queryFn: () => sysadmin.listOrgs() });

  const suspend = useMutation({
    mutationFn: (id: string) => sysadmin.suspendOrg(id),
    onMutate: (id) => flip(qc, ["sysadmin", "orgs"], id, { status: "suspended" }),
    onError: () => toast("Couldn't reach the backend.", "alert"),
    onSuccess: () => toast("Organization suspended.", "check"),
  });
  const restore = useMutation({
    mutationFn: (id: string) => sysadmin.restoreOrg(id),
    onMutate: (id) => flip(qc, ["sysadmin", "orgs"], id, { status: "active" }),
    onError: () => toast("Couldn't reach the backend.", "alert"),
    onSuccess: () => toast("Organization restored.", "check"),
  });

  const orgs: Organization[] = data ?? [];
  return (
    <section className="card" style={{ marginTop: "var(--space-5)" }}>
      <div className="card-head">
        <h2 className="card-title">Organizations</h2>
        <button className="btn btn-ghost btn-sm" onClick={() => toast("Creating orgs isn't implemented yet.", "alert")}>
          + New org
        </button>
      </div>
      <table className="table">
        <thead>
          <tr>
            <th>Organization</th>
            <th>Plan</th>
            <th>Workspaces</th>
            <th>Seats</th>
            <th>Status</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {orgs.map((o) => (
            <tr key={o.id}>
              <td>
                <div style={{ fontWeight: 600 }}>{o.name}</div>
                {o.subdomain && <div className="muted mono" style={{ fontSize: 12 }}>{o.subdomain}</div>}
              </td>
              <td>
                <span className={`role-pill ${o.plan === "enterprise" || o.plan === "team" ? "role-admin" : "role-member"}`}>
                  {o.plan}
                </span>
              </td>
              <td>{o.workspace_count}</td>
              <td className="mono">{o.seats_used} / {o.seats_total}</td>
              <td>
                <Badge tone={o.status === "active" ? "success" : o.status === "trial" ? "warn" : "danger"} dot>
                  {o.status}
                </Badge>
              </td>
              <td>
                {o.status === "suspended" ? (
                  <button className="btn btn-ghost btn-sm" onClick={() => restore.mutate(o.id)}>Restore</button>
                ) : (
                  <button className="btn btn-ghost btn-sm" onClick={() => suspend.mutate(o.id)}>Suspend</button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

/* ── Sign-up requests ───────────────────────────────────────────── */
function SignupRequestsTable() {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data } = useQuery({ queryKey: ["sysadmin", "requests"], queryFn: () => sysadmin.listSignupRequests() });
  const approve = useMutation({
    mutationFn: (id: string) => sysadmin.approveSignup(id),
    onMutate: (id) => {
      const prev = qc.getQueryData<SignupRequest[]>(["sysadmin", "requests"]);
      qc.setQueryData<SignupRequest[]>(["sysadmin", "requests"], (l) => (l ?? []).filter((r) => r.id !== id));
      return { prev };
    },
    onSuccess: () => toast("Request approved.", "check"),
    onError: (_e, _id, ctx) => {
      if (ctx?.prev) qc.setQueryData(["sysadmin", "requests"], ctx.prev);
      toast("Couldn't reach the backend.", "alert");
    },
  });
  const list: SignupRequest[] = data ?? [];
  return (
    <section className="card" style={{ marginTop: "var(--space-5)" }}>
      <div className="card-head">
        <h2 className="card-title">Sign-up requests</h2>
        {list.length > 0 && (
          <Badge tone="warn" dot>{list.length} pending</Badge>
        )}
      </div>
      {list.length === 0 ? (
        <div className="muted" style={{ padding: "var(--space-5)" }}>No pending requests.</div>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>User</th>
              <th>Workspace</th>
              <th>Requested</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {list.map((r) => (
              <tr key={r.id}>
                <td>
                  <div style={{ fontWeight: 600 }}>{r.name}</div>
                  <div className="muted mono" style={{ fontSize: 12 }}>{r.email}</div>
                </td>
                <td>{r.workspace_name ?? "—"}</td>
                <td className="muted">{timeAgo(r.requested_at)}</td>
                <td>
                  <button className="btn btn-primary btn-sm" onClick={() => approve.mutate(r.id)}>Approve</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

/* ── helpers ────────────────────────────────────────────────────── */
function flip(
  qc: ReturnType<typeof useQueryClient>,
  key: unknown[],
  id: string,
  patch: Partial<Organization>,
) {
  const prev = qc.getQueryData<Organization[]>(key);
  qc.setQueryData<Organization[]>(key, (l) => (l ?? []).map((o) => (o.id === id ? { ...o, ...patch } : o)));
  return { prev };
}

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const min = Math.round((Date.now() - then) / 60000);
  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr}h ago`;
  return `${Math.round(hr / 24)}d ago`;
}
