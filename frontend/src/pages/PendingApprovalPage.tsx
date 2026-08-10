import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { auth } from "../api/client";
import { useAuth } from "../store/auth";
import { useToast } from "../components/ui";
import { Icon } from "../lib/icons";

export function PendingApprovalPage() {
  const user = useAuth((s) => s.user);
  const activeWorkspace = useAuth((s) => s.activeWorkspace);
  const { toast } = useToast();

  // Poll the signup status (every 10s) so the page updates when an admin
  // approves. Degrades gracefully when the backend is absent.
  const { data } = useQuery({
    queryKey: ["auth", "signup-status"],
    queryFn: () => auth.signupStatus(),
    retry: 0,
    refetchInterval: 10_000,
  });

  const email = data?.email ?? user?.email ?? "your email";
  const workspaceName = data?.workspace_name ?? activeWorkspace?.name ?? "your workspace";
  const adminName = data?.admin_name ?? "your workspace admin";

  // If the backend reports approval, send the user to sign in.
  const approved = data?.state === "approved";

  const steps = [
    { label: "Account created", sub: email, state: "done" as const },
    { label: "Join request submitted", sub: `Target workspace: ${workspaceName}`, state: "done" as const },
    { label: "Admin review", sub: `${adminName} will approve your access`, state: "current" as const },
    { label: "Access granted", sub: "You'll be able to sign in", state: "pending" as const },
  ];

  return (
    <div className="pending-wrap center stack" style={{ maxWidth: 520, margin: "0 auto", padding: "var(--space-12) var(--space-6)" }}>
      <div className="pending-icon">
        <Icon name="clock" size={34} />
      </div>
      <h1>Request sent — awaiting approval</h1>
      <p className="lead">
        We've notified <b>{adminName}</b> about your request to join <b>{workspaceName}</b> ({email}). You'll get an
        email as soon as you're approved.
      </p>

      <div className="pending-steps">
        {steps.map((s) => (
          <div key={s.label} className={`pending-step ${s.state}`}>
            <span className="ps-dot">
              {s.state === "done" ? <Icon name="check" size={14} /> : s.state === "current" ? "3" : "4"}
            </span>
            <span className="ps-text">
              <b>{s.label}</b>
              <span>{s.sub}</span>
            </span>
          </div>
        ))}
      </div>

      {approved && (
        <div
          className="card flush"
          style={{
            marginBottom: "var(--space-4)",
            borderColor: "var(--success)",
            color: "var(--fg-2)",
            textAlign: "center",
          }}
        >
          ✅ You've been approved — you can sign in now.
        </div>
      )}

      <div className="row" style={{ justifyContent: "center", gap: "var(--space-3)" }}>
        {!approved && (
          <button
            type="button"
            className="btn btn-ghost"
            onClick={() => {
              auth
                .resendSignupNotification()
                .catch(() => undefined)
                .finally(() => toast("We'll email you when approved.", "bell"));
            }}
          >
            Resend notification
          </button>
        )}
        <Link to="/login" className="btn btn-primary">
          {approved ? "Sign in" : "Back to sign in"}
        </Link>
      </div>

      <p className="muted" style={{ fontSize: 12, marginTop: "var(--space-6)" }}>
        Need to start over? <Link to="/signup">Resubmit your request</Link> or contact your admin.
      </p>
    </div>
  );
}
