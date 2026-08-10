import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { workspaces } from "../../api/client";
import { useAuth } from "../../store/auth";
import { Field, Input, Modal, Select, useToast } from "../ui";

const FORM_ID = "new-workspace-form";

export function NewWorkspaceModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const setActiveWorkspace = useAuth((s) => s.setActiveWorkspace);
  const hydrate = useAuth((s) => s.hydrate);
  const current = useAuth((s) => s.activeWorkspace);
  const user = useAuth((s) => s.user);
  const qc = useQueryClient();
  const { toast } = useToast();
  const navigate = useNavigate();

  const [name, setName] = useState("");
  const [repo, setRepo] = useState("github.com/hoangdaochuz/payments-svc");
  const [defaultBranch, setDefaultBranch] = useState("main");
  const [role, setRole] = useState<"owner" | "admin">("owner");

  const connectingNewRepo = repo === "";

  const mutation = useMutation({
    mutationFn: () => workspaces.create({ name, repo_source: repo, default_branch: defaultBranch, role }),
    onSuccess: (ws) => {
      const workspacesList = [...(useAuth.getState().workspaces ?? []), ws];
      if (user) hydrate({ user, workspaces: workspacesList, active_workspace_id: ws.id });
      setActiveWorkspace(ws.id);
      qc.invalidateQueries({ queryKey: ["workspaces"] });
      toast("Workspace created — connect agents next", "check");
      onClose();
      // Spec: on success the new workspace becomes active → go to its dashboard.
      navigate("/dashboard");
    },
    onError: () => toast("Couldn't reach the backend. Endpoints aren't implemented yet.", "alert"),
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="New workspace"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button
            type="submit"
            form={FORM_ID}
            className="btn btn-primary"
            disabled={!name || connectingNewRepo || mutation.isPending}
          >
            {mutation.isPending ? "Creating…" : "Create workspace"}
          </button>
        </>
      }
    >
      <form
        id={FORM_ID}
        className="stack"
        style={{ padding: "var(--space-5)" }}
        onSubmit={(e) => {
          e.preventDefault();
          mutation.mutate();
        }}
      >
        <Field label="Workspace name">
          <Input required value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. Payments Service" />
        </Field>
        <Field
          label="Repository"
          help={
            connectingNewRepo
              ? "Connecting a new repo isn't implemented yet — pick an existing repo to continue."
              : "Agents will only see files inside this repo."
          }
        >
          <Select value={repo} onChange={(e) => setRepo(e.target.value)}>
            <option value="github.com/hoangdaochuz/payments-svc">github.com/hoangdaochuz/payments-svc</option>
            <option value="github.com/acme/billing">github.com/acme/billing</option>
            <option value="">Connect a new repo…</option>
          </Select>
        </Field>
        <Field label="Default branch">
          <Input className="mono" value={defaultBranch} onChange={(e) => setDefaultBranch(e.target.value)} />
        </Field>
        <Field label="Your role">
          <Select value={role} onChange={(e) => setRole(e.target.value as "owner" | "admin")}>
            <option value="owner">Owner</option>
            <option value="admin">Admin</option>
          </Select>
        </Field>
        {current && (
          <p className="muted" style={{ fontSize: 12 }}>
            Switching from <b>{current.name}</b>.
          </p>
        )}
      </form>
    </Modal>
  );
}
