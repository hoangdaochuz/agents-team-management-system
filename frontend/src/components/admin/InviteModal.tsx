import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { invites } from "../../api/client";
import type { Role } from "../../api/types";
import { Field, Modal, Segmented, Textarea, useToast } from "../ui";

const FORM_ID = "invite-form";

export function InviteModal({ workspaceId, open, onClose }: { workspaceId: string | undefined; open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [emails, setEmails] = useState("");
  const [role, setRole] = useState<Role>("member");

  const mutation = useMutation({
    mutationFn: () => invites.send(workspaceId!, { emails: parseEmails(emails), role }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["members", workspaceId] });
      toast("Invitations sent", "check");
      setEmails("");
      onClose();
    },
    onError: () => toast("Couldn't reach the backend. Endpoints aren't implemented yet.", "alert"),
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Invite people"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" form={FORM_ID} className="btn btn-primary" disabled={!emails.trim() || mutation.isPending}>
            {mutation.isPending ? "Sending…" : "Send invites"}
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
        <Field label="Email addresses" help="Separate multiple addresses with commas or new lines.">
          <Textarea
            rows={3}
            required
            value={emails}
            onChange={(e) => setEmails(e.target.value)}
            placeholder="teammate@company.com, another@company.com"
          />
        </Field>
        <Field label="Role" help="Members can run tasks & review; Admins can manage agents, skills, and members.">
          <Segmented<Role>
            value={role}
            onChange={setRole}
            options={[
              { value: "member", label: "Member" },
              { value: "admin", label: "Admin" },
            ]}
          />
        </Field>
      </form>
    </Modal>
  );
}

function parseEmails(s: string): string[] {
  return s
    .split(/[\s,]+/)
    .map((e) => e.trim())
    .filter(Boolean);
}
