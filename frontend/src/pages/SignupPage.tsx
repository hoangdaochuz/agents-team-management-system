import { useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { useMutation } from "@tanstack/react-query";
import { ApiError, auth } from "../api/client";
import { useAuth } from "../store/auth";
import { useToast } from "../components/ui";
import { Icon } from "../lib/icons";
import { AuthShell } from "./LoginPage";

type StartMode = "join" | "create";

export function SignupPage() {
  const navigate = useNavigate();
  const status = useAuth((s) => s.status);
  const { toast } = useToast();

  const [fullName, setFullName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [startMode, setStartMode] = useState<StartMode>("join");
  const [inviteCode, setInviteCode] = useState("");
  const [orgName, setOrgName] = useState("");
  const [agree, setAgree] = useState(false);

  const mutation = useMutation({
    mutationFn: () =>
      auth.signup({
        full_name: fullName,
        email,
        password,
        start_mode: startMode,
        invite_code: startMode === "join" ? inviteCode : undefined,
        organization_name: startMode === "create" ? orgName : undefined,
      }),
    onSuccess: () => {
      navigate("/pending", { replace: true });
    },
    onError: (e: unknown) => {
      if (e instanceof ApiError) toast(e.message, "alert");
      else toast("Couldn't reach the backend. Endpoints aren't implemented yet.", "alert");
    },
  });

  if (status === "authenticated") return <Navigate to="/dashboard" replace />;

  const canSubmit =
    agree && fullName && email && password.length >= 12 && (startMode === "join" ? inviteCode : orgName);

  return (
    <AuthShell pitchTitle="Spin up an AI agent team for your repo.">
      <h1>Create your account</h1>
      <p className="muted">Request access — an admin will review and invite you.</p>

      <div className="field" style={{ marginTop: "var(--space-5)" }}>
        <span>How do you want to start?</span>
        <div className="seg">
          <button
            type="button"
            className={startMode === "join" ? "active" : ""}
            role="tab"
            aria-selected={startMode === "join"}
            onClick={() => setStartMode("join")}
          >
            Join a workspace
          </button>
          <button
            type="button"
            className={startMode === "create" ? "active" : ""}
            role="tab"
            aria-selected={startMode === "create"}
            onClick={() => setStartMode("create")}
          >
            Create new org
          </button>
        </div>
      </div>

      <form
        className="auth-form"
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
      >
        <label className="field">
          <span>Full name</span>
          <input
            className="input"
            required
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
            placeholder="Jane Engineer"
          />
        </label>
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
            placeholder="At least 12 characters"
          />
          <span className="field-help">Use 12+ characters with a number and symbol.</span>
        </label>

        {startMode === "join" ? (
          <label className="field">
            <span>Invite code or URL</span>
            <input
              className="input"
              value={inviteCode}
              onChange={(e) => setInviteCode(e.target.value)}
              placeholder="e.g. agentops-invite-7F3A"
            />
            <span className="field-help">Ask your workspace admin for an invite code, or paste the invite URL.</span>
          </label>
        ) : (
          <label className="field">
            <span>Organization name</span>
            <input
              className="input"
              value={orgName}
              onChange={(e) => setOrgName(e.target.value)}
              placeholder="e.g. Acme Inc."
            />
            <span className="field-help">You'll be the Owner of the first workspace in this organization.</span>
          </label>
        )}

        <label className="remember">
          <input type="checkbox" checked={agree} onChange={(e) => setAgree(e.target.checked)} />I agree to the{" "}
          <a href="#terms" onClick={(e) => e.preventDefault()}>
            Terms &amp; Privacy Policy
          </a>
        </label>

        <button type="submit" className="btn btn-primary w-full" disabled={!canSubmit || mutation.isPending}>
          {mutation.isPending ? "Sending request…" : "Request access"}
        </button>
      </form>

      <div className="auth-divider">OR</div>

      <div className="stack">
        <button
          type="button"
          className="btn btn-soft w-full"
          onClick={() => {
            auth
              .ssoBegin("google")
              .catch(() => undefined)
              .finally(() => {
                toast("SSO is not connected yet (backend pending).", "alert");
                navigate("/pending");
              });
          }}
        >
          <Icon name="github" size={16} /> Continue with Google Workspace
        </button>
        <button
          type="button"
          className="btn btn-soft w-full"
          onClick={() => {
            auth
              .ssoBegin("saml")
              .catch(() => undefined)
              .finally(() => {
                toast("SSO is not connected yet (backend pending).", "alert");
                navigate("/pending");
              });
          }}
        >
          <Icon name="lock" size={16} /> Continue with SAML (SSO)
        </button>
      </div>

      <p className="muted center" style={{ marginTop: "var(--space-6)" }}>
        Already have one? <Link to="/login">Sign in</Link>
      </p>
    </AuthShell>
  );
}
