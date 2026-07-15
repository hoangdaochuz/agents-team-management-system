import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { tasks } from "../../api/client";
import { Badge, Button, Field, Input, Modal, Segmented, Textarea, useToast } from "../ui";

type Verdict = "approve" | "changes";

const TAG_SUGGESTIONS = ["good quality", "needs more tests", "breaking change"];

export function ReviewModal({
  open,
  onClose,
  taskId,
}: {
  open: boolean;
  onClose: () => void;
  taskId: string;
}) {
  const { toast } = useToast();
  const [verdict, setVerdict] = useState<Verdict>("approve");
  const [note, setNote] = useState("");
  const [tags, setTags] = useState<string[]>([]);

  const reset = () => {
    setVerdict("approve");
    setNote("");
    setTags([]);
  };

  const approve = useMutation({
    mutationFn: () => tasks.patchStatus(taskId, "done"),
    onSuccess: () => {
      toast("Review accepted · task closed");
      reset();
      onClose();
    },
    onError: (e: unknown) => toast(e instanceof Error ? e.message : "Couldn't submit review", "alert"),
  });

  const rerun = useMutation({
    mutationFn: () => tasks.rerunTask(taskId),
    onSuccess: () => {
      toast("Changes requested · agent re-running with your feedback");
      reset();
      onClose();
    },
    onError: (e: unknown) => toast(e instanceof Error ? e.message : "Couldn't request changes", "alert"),
  });

  const submit = () => {
    if (verdict === "approve") {
      approve.mutate();
    } else {
      rerun.mutate();
    }
  };

  const submitting = approve.isPending || rerun.isPending;

  const toggleTag = (t: string) =>
    setTags((prev) => (prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]));

  return (
    <Modal
      open={open}
      onClose={() => {
        reset();
        onClose();
      }}
      title="Review output"
      footer={
        <>
          <Button
            variant="ghost"
            onClick={() => {
              reset();
              onClose();
            }}
          >
            Later
          </Button>
          <Button variant="primary" onClick={submit} disabled={submitting}>
            {verdict === "approve" ? "Accept & close" : "Request changes"}
          </Button>
        </>
      }
    >
      <div style={{ marginBottom: "var(--space-2)" }}>
        <Badge tone="accent" dot>
          after completion
        </Badge>
      </div>
      <p className="muted" style={{ fontSize: 13, marginTop: 0 }}>
        Accept to close the task, or request changes — the agent starts a new run applying your feedback.
      </p>

      <Field label="Decision">
        <Segmented<Verdict>
          value={verdict}
          onChange={setVerdict}
          options={[
            { value: "approve", label: "✓ Accept" },
            { value: "changes", label: "Request changes" },
          ]}
        />
      </Field>

      {verdict === "changes" && (
        <Field label="Feedback / change request" help="The agent re-runs with this feedback.">
          <Textarea
            placeholder="e.g. add a proper test case for expired tokens"
            value={note}
            onChange={(e) => setNote((e.target as HTMLTextAreaElement).value)}
          />
        </Field>
      )}

      <Field label="Tags">
        <div className="row" style={{ flexWrap: "wrap", gap: 6 }}>
          {TAG_SUGGESTIONS.map((t) => (
            <span
              key={t}
              className="tag clickable"
              onClick={() => toggleTag(t)}
              style={{
                cursor: "pointer",
                opacity: tags.includes(t) ? 1 : 0.6,
                borderColor: tags.includes(t) ? "var(--accent)" : undefined,
                color: tags.includes(t) ? "var(--accent-active)" : undefined,
              }}
            >
              {t}
            </span>
          ))}
        </div>
        <Input placeholder="Add a tag…" style={{ marginTop: 6 }} />
      </Field>
    </Modal>
  );
}
