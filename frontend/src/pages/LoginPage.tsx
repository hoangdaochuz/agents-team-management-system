import { useState } from "react";
import { Link, Navigate, useLocation, useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { ApiError, auth } from "../api/client";
import { useAuth } from "../store/auth";
import { useToast } from "../components/ui";
import { Icon } from "../lib/icons";

export function LoginPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const loginSession = useAuth((s) => s.login);
  const status = useAuth((s) => s.status);
  const { toast } = useToast();
  const from = (location.state as { from?: string } | null)?.from ?? "/dashboard";

  const [email, setEmail] = useState("dang@agentops.dev");
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(true);
  const [errMsg, setErrMsg] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => auth.login({ email, password, remember }),
    onSuccess: (session) => {
      loginSession(session);
      navigate(from, { replace: true });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError) {
        setErrMsg(e.status === 401 ? "Incorrect email or password." : e.message);
      } else {
        setErrMsg("Couldn't reach the backend. Endpoints aren't implemented yet.");
      }
    },
  });

  if (status === "authenticated") {
    return <Navigate to={from} replace />;
  }

  const onSso = (provider: "google" | "saml") => {
    auth
      .ssoBegin(provider)
      .catch(() => undefined)
      .finally(() => toast("SSO is not connected yet (backend pending).", "alert"));
  };

  return (
    <AuthShell pitchTitle="One control room for every AI agent team.">
      <h1>Sign in</h1>
      <p className="muted">Welcome back — sign in to your workspace.</p>

      {errMsg && (
        <div
          style={{
            marginTop: "var(--space-4)",
            padding: "var(--space-2) var(--space-3)",
            borderRadius: "var(--radius-md)",
            border: "1px solid var(--border)",
            background: "var(--surface-warm)",
            color: "var(--fg-2)",
            fontSize: "var(--text-sm)",
          }}
        >
          {errMsg}
        </div>
      )}

      <form
        className="auth-form"
        onSubmit={(e) => {
          e.preventDefault();
          setErrMsg(null);
          mutation.mutate();
        }}
      >
        <label className="field">
          <span>Work email</span>
          <input
            className="input"
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@company.com"
          />
        </label>
        <label className="field">
          <span>Password</span>
          <input
            className="input"
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="••••••••••"
          />
        </label>
        <div className="spread">
          <label className="remember">
            <input type="checkbox" checked={remember} onChange={(e) => setRemember(e.target.checked)} /> Keep me
            signed in
          </label>
          <a className="muted" href="#forgot" onClick={(e) => e.preventDefault()}>
            Forgot password?
          </a>
        </div>

        <button type="submit" className="btn btn-primary w-full" disabled={mutation.isPending}>
          {mutation.isPending ? "Signing in…" : "Sign in"}
        </button>
      </form>

      <div className="auth-divider">OR</div>

      <div className="stack">
        <button type="button" className="btn btn-soft w-full" onClick={() => onSso("google")}>
          <Icon name="github" size={16} /> Continue with Google Workspace
        </button>
        <button type="button" className="btn btn-soft w-full" onClick={() => onSso("saml")}>
          <Icon name="lock" size={16} /> Continue with SAML (SSO)
        </button>
      </div>

      <p className="muted center" style={{ marginTop: "var(--space-6)" }}>
        New here? <Link to="/signup">Create an organization</Link>
      </p>
    </AuthShell>
  );
}

/** Shared split-screen auth layout (mirrors prototype `.auth`). */
export function AuthShell({
  pitchTitle,
  children,
}: {
  pitchTitle: string;
  children: React.ReactNode;
}) {
  return (
    <div className="auth">
      <aside className="auth-aside">
        <div className="auth-brand">
          <div className="brand-mark">◆</div>
          <div className="brand-name" style={{ color: "#fff" }}>
            Agent Ops
          </div>
        </div>
        <div className="auth-pitch">
          <h2>{pitchTitle}</h2>
          <p>SOC 2 controls, SSO/SAML, per-workspace isolation, and credential-less task sandboxes.</p>
        </div>
        <div className="auth-foot">© Agent Ops · Built for teams shipping with autonomous agents</div>
      </aside>
      <main className="auth-main">
        <div className="auth-card">{children}</div>
      </main>
    </div>
  );
}
