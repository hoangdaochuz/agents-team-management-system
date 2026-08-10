import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { audit, invites, members } from "../api/client";
import type { AuditEntry, Member, Role, SignupRequest } from "../api/types";
import { useActiveWorkspaceId } from "../lib/workspace";
import { Avatar } from "../components/ui/Avatar";
import { Badge } from "../components/ui/Badge";
import { Button } from "../components/ui/Button";
import { Tabs, type TabDef } from "../components/ui/Tabs";
import { useToast } from "../components/ui/Toast";
import { InviteModal } from "../components/admin/InviteModal";
import { WorkspaceScopeBanner } from "../components/workspaces/WorkspaceScopeBanner";

const ROLES: Role[] = ["owner", "admin", "member"];

const CAPABILITIES: { label: string; grants: Record<Role, boolean> }[] = [
  { label: "View dashboard", grants: { owner: true, admin: true, member: true } },
  { label: "Create tasks", grants: { owner: true, admin: true, member: true } },
  { label: "Give feedback", grants: { owner: true, admin: true, member: true } },
  { label: "Create/edit agents", grants: { owner: true, admin: true, member: false } },
  { label: "Manage skills", grants: { owner: true, admin: true, member: false } },
  { label: "Approve PRs", grants: { owner: true, admin: true, member: false } },
  { label: "Invite members", grants: { owner: true, admin: true, member: false } },
  { label: "View audit log", grants: { owner: true, admin: false, member: false } },
  { label: "Delete workspace", grants: { owner: true, admin: false, member: false } },
];

export function AdminPage() {
  const wid = useActiveWorkspaceId();
  const [inviting, setInviting] = useState(false);

  const tabs: TabDef[] = [
    { key: "members", label: "Members", content: <MembersTab wid={wid} onInvite={() => setInviting(true)} /> },
    { key: "roles", label: "Roles & permissions", content: <RolesTab /> },
    { key: "audit", label: "Audit log", content: <AuditTab wid={wid} /> },
  ];

  return (
    <>
      <div className="crumbs">
        <Link to="/workspaces">Workspaces</Link>
        <span className="sep">/</span>
        <span>Members &amp; roles</span>
      </div>

      <div className="page-head">
        <div>
          <h1 className="page-title">Members &amp; roles</h1>
          <div className="page-sub">Manage who can access this workspace and what they can do</div>
        </div>
        <Button variant="primary" onClick={() => setInviting(true)}>
          + Invite people
        </Button>
      </div>

      <div className="banner" style={{ display: "flex", gap: "var(--space-3)", alignItems: "center", padding: "var(--space-3) var(--space-4)", border: "1px solid var(--border-soft)", borderRadius: "var(--radius-md)", marginBottom: "var(--space-5)" }}>
        <Badge tone="accent">Owner only</Badge>
        <span className="muted" style={{ fontSize: "var(--text-sm)" }}>
          Only workspace owners and admins see this screen.
        </span>
      </div>

      <WorkspaceScopeBanner />
      <PendingApprovalsCard wid={wid} />

      <section className="card">
        <Tabs tabs={tabs} />
      </section>

      <InviteModal workspaceId={wid} open={inviting} onClose={() => setInviting(false)} />
    </>
  );
}

/* ── Pending approvals ─────────────────────────────────────────── */
function PendingApprovalsCard({ wid }: { wid: string | undefined }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["pending", wid],
    queryFn: () => invites.listPending(wid!),
    enabled: Boolean(wid),
  });
  const list = data ?? [];

  const approve = useMutation({
    mutationFn: (id: string) => invites.approve(wid!, id),
    onMutate: (id) => {
      const prev = qc.getQueryData<SignupRequest[]>(["pending", wid]);
      qc.setQueryData<SignupRequest[]>(["pending", wid], (l) => (l ?? []).filter((r) => r.id !== id));
      return { prev };
    },
    onSuccess: () => toast("Approved · invite sent", "check"),
    onError: (_e, _id, ctx) => {
      if (ctx?.prev) qc.setQueryData(["pending", wid], ctx.prev);
      toast("Couldn't reach the backend.", "alert");
    },
  });
  const decline = useMutation({
    mutationFn: (id: string) => invites.decline(wid!, id),
    onMutate: (id) => {
      const prev = qc.getQueryData<SignupRequest[]>(["pending", wid]);
      qc.setQueryData<SignupRequest[]>(["pending", wid], (l) => (l ?? []).filter((r) => r.id !== id));
      return { prev };
    },
    onSuccess: () => toast("Request declined", "check"),
    onError: (_e, _id, ctx) => {
      if (ctx?.prev) qc.setQueryData(["pending", wid], ctx.prev);
      toast("Couldn't reach the backend.", "alert");
    },
  });

  if (isLoading || isError) return null;
  if (list.length === 0) return null;

  return (
    <section className="card flush" style={{ marginBottom: "var(--space-5)" }}>
      <div className="card-head">
        <h2 className="card-title">Pending approvals</h2>
        <Badge tone="warn" dot>{list.length} pending</Badge>
      </div>
      <table className="table">
        <thead>
          <tr>
            <th>Requester</th>
            <th>Requested role</th>
            <th>Requested</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.map((r) => (
            <tr key={r.id}>
              <td>
                <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "center" }}>
                  <Avatar name={r.name} />
                  <div>
                    <div style={{ fontWeight: 600 }}>{r.name}</div>
                    <div className="muted mono" style={{ fontSize: 12 }}>{r.email}</div>
                  </div>
                </div>
              </td>
              <td>
                <span className={`role-pill role-${r.requested_role}`}>{r.requested_role}</span>
              </td>
              <td className="muted">{timeAgo(r.requested_at)}</td>
              <td>
                <div className="row" style={{ gap: "var(--space-2)", justifyContent: "flex-end" }}>
                  <Button variant="ghost" size="sm" onClick={() => decline.mutate(r.id)}>Decline</Button>
                  <Button variant="primary" size="sm" onClick={() => approve.mutate(r.id)}>Approve</Button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

/* ── Members tab ────────────────────────────────────────────────── */
function MembersTab({ wid, onInvite }: { wid: string | undefined; onInvite: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["members", wid],
    queryFn: () => members.list(wid!),
    enabled: Boolean(wid),
  });

  const updateRole = useMutation({
    mutationFn: ({ id, role }: { id: string; role: Role }) => members.updateRole(wid!, id, role),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["members", wid] });
      toast("Role updated", "check");
    },
    onError: () => toast("Couldn't reach the backend.", "alert"),
  });

  if (isLoading || isError) {
    return <div className="muted" style={{ padding: "var(--space-5)" }}>Loading… (endpoints aren't implemented yet.)</div>;
  }

  const list: Member[] = data ?? [];
  const seatsUsed = list.filter((m) => !m.is_service_account).length;

  return (
    <>
      <table className="table">
        <thead>
          <tr>
            <th>Member</th>
            <th>Role</th>
            <th>Status</th>
            <th>Last active</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {list.map((m) => (
            <tr key={m.id}>
              <td>
                <div style={{ display: "flex", gap: "var(--space-2)", alignItems: "center" }}>
                  <Avatar name={m.is_service_account ? "SA" : m.user.name} />
                  <div>
                    <div style={{ fontWeight: 600 }}>{m.is_service_account ? "Service account" : m.user.name}</div>
                    <div className="muted mono" style={{ fontSize: 12 }}>{m.user.email}</div>
                  </div>
                </div>
              </td>
              <td>
                <select
                  className="input"
                  style={{ width: "auto" }}
                  value={m.role}
                  onChange={(e) => updateRole.mutate({ id: m.id, role: e.target.value as Role })}
                >
                  {ROLES.map((r) => (
                    <option key={r} value={r}>{r}</option>
                  ))}
                </select>
              </td>
              <td>
                <Badge tone={m.status === "active" ? "success" : m.status === "invited" ? "warn" : "muted"} dot>
                  {m.status}
                </Badge>
              </td>
              <td className="muted">{m.last_active_at ? timeAgo(m.last_active_at) : "—"}</td>
              <td>
                <button className="icon-btn" aria-label="Member actions">
                  ⋯
                </button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="muted" style={{ fontSize: 12, padding: "var(--space-3) 0" }}>
        Seats: <b>{seatsUsed} / 10</b> used on the Pro plan. Service accounts don't count toward seats.{" "}
        <a href="#invite" onClick={(e) => { e.preventDefault(); onInvite(); }}>Invite more</a>
      </div>
    </>
  );
}

/* ── Roles matrix ───────────────────────────────────────────────── */
function RolesTab() {
  return (
    <table className="matrix table">
      <thead>
        <tr>
          <th>Capability</th>
          {ROLES.map((r) => (
            <th key={r} style={{ textTransform: "capitalize" }}>{r}</th>
          ))}
        </tr>
      </thead>
      <tbody>
        {CAPABILITIES.map((c) => (
          <tr key={c.label}>
            <td>{c.label}</td>
            {ROLES.map((r) => (
              <td key={r} className={c.grants[r] ? "yes" : "no"}>
                {c.grants[r] ? <Check size={16} /> : "—"}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/* ── Audit log ──────────────────────────────────────────────────── */
function AuditTab({ wid }: { wid: string | undefined }) {
  const { toast } = useToast();
  const { data, isLoading, isError } = useQuery({
    queryKey: ["audit", wid],
    queryFn: () => audit.list(wid!),
    enabled: Boolean(wid),
  });

  const exportMut = useMutation({
    mutationFn: () => audit.exportLog(wid!),
    onSuccess: () => toast("Export queued — we'll email the file.", "check"),
    onError: () => toast("Couldn't reach the backend.", "alert"),
  });

  const list: AuditEntry[] = data ?? [];

  return (
    <>
      <div className="spread" style={{ marginBottom: "var(--space-3)" }}>
        <div className="muted" style={{ fontSize: "var(--text-sm)" }}>{list.length} entries</div>
        <Button variant="ghost" size="sm" onClick={() => exportMut.mutate()}>
          Export audit log
        </Button>
      </div>
      {(isLoading || isError) && (
        <div className="muted" style={{ padding: "var(--space-5)" }}>Loading… (endpoints aren't implemented yet.)</div>
      )}
      {!isLoading && !isError && (
        <table className="table">
          <thead>
            <tr>
              <th>Actor</th>
              <th>Action</th>
              <th>Target</th>
              <th>Time</th>
              <th>IP</th>
            </tr>
          </thead>
          <tbody>
            {list.map((e) => (
              <tr key={e.id}>
                <td>{e.actor.name}</td>
                <td>
                  <Badge tone="muted">{e.action_kind ?? "event"}</Badge> {e.action}
                </td>
                <td className="muted">{e.target ?? "—"}</td>
                <td className="muted">{timeAgo(e.created_at)}</td>
                <td className="mono muted">{e.ip ?? "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

/* small inline check icon to avoid an extra import collision */
function Check({ size }: { size: number }) {
  return (
    <svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true" style={{ display: "inline-block", color: "var(--success)", verticalAlign: "middle" }}>
      <path d="M20 6 9 17l-5-5" fill="none" stroke="currentColor" strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function timeAgo(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return iso;
  const diff = Date.now() - then;
  const min = Math.round(diff / 60000);
  if (min < 1) return "just now";
  if (min < 60) return `${min}m ago`;
  const hr = Math.round(min / 60);
  if (hr < 24) return `${hr}h ago`;
  const day = Math.round(hr / 24);
  return `${day}d ago`;
}
