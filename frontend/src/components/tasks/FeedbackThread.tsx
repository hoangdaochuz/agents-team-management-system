import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { feedback } from "../../api/client";
import type { Feedback } from "../../api/types";
import { relativeTime } from "../../lib/format";
import { Avatar, AsyncBoundary, Button, Card, CardHead, Textarea, useToast } from "../ui";
import { Icon } from "../../lib/icons";

export function FeedbackThread({ taskId }: { taskId: string }) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const [body, setBody] = useState("");

  const query = useQuery({
    queryKey: ["feedback", taskId],
    queryFn: () => feedback.listFeedback(taskId),
  });

  const add = useMutation({
    mutationFn: (text: string) => feedback.addFeedback(taskId, text),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["feedback", taskId] });
      setBody("");
      toast("Feedback sent");
    },
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : "Couldn't send feedback";
      toast(msg, "alert");
    },
  });

  const items: Feedback[] = query.data ?? [];

  const send = () => {
    const text = body.trim();
    if (!text) {
      toast("Type a note first", "send");
      return;
    }
    add.mutate(text);
  };

  return (
    <Card>
      <CardHead title={<h2 className="card-title">Feedback log</h2>} />

      <AsyncBoundary
        isLoading={query.isLoading}
        isError={query.isError}
        error={query.error}
        data={items}
        isEmpty={(d) => d.length === 0}
        emptyTitle="No feedback yet"
        emptyHint="Notes you leave here guide the agent on the next run."
      >
        {(data) => (
          <div className="stack" style={{ gap: "var(--space-3)" }}>
            {data.map((f) => (
              <div className="fb-item" key={f.id}>
                <Avatar name={f.author === "user" ? "You" : "Agent"} size="sm" />
                <div className="fb-bubble">
                  <div className="fb-meta">
                    {f.author === "user" ? "You" : "Agent"} · {relativeTime(f.created_at)}
                  </div>
                  <div style={{ fontSize: 14 }}>{f.body}</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </AsyncBoundary>

      <div className="field mt-6" style={{ gap: 8 }}>
        <Textarea
          placeholder="e.g. keep backward-compat for the old Parse()"
          value={body}
          onChange={(e) => setBody((e.target as HTMLTextAreaElement).value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
              e.preventDefault();
              send();
            }
          }}
        />
        <div className="spread">
          <span className="muted" style={{ fontSize: 12 }}>
            ⏎ Cmd+Enter to send
          </span>
          <Button variant="primary" size="sm" icon={<Icon name="send" size={14} />} onClick={send} disabled={add.isPending}>
            Send
          </Button>
        </div>
      </div>
    </Card>
  );
}
