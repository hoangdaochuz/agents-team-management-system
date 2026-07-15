import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { feedback, runs, tasks } from "../api/client";
import type { Artifact } from "../api/types";
import { useTaskStream } from "../hooks/useTaskStream";
import { shortId } from "../lib/format";
import {
  AsyncBoundary,
  Badge,
  Button,
  Card,
  Field,
  Tabs,
  Textarea,
  useToast,
} from "../components/ui";
import { Icon } from "../lib/icons";
import { TaskHeader } from "../components/tasks/TaskHeader";
import { Terminal } from "../components/tasks/Terminal";
import { StepsList } from "../components/tasks/StepsList";
import { FeedbackThread } from "../components/tasks/FeedbackThread";
import { ReviewModal } from "../components/tasks/ReviewModal";

export function TaskDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const qc = useQueryClient();
  const { toast } = useToast();

  const [paused, setPaused] = useState(false);
  const [pausedBody, setPausedBody] = useState("");
  const [reviewOpen, setReviewOpen] = useState(false);

  const taskQuery = useQuery({
    queryKey: ["task", id],
    queryFn: () => tasks.getTask(id),
    retry: false,
  });

  const { steps, connected } = useTaskStream(id);

  const artifactsQuery = useQuery({
    queryKey: ["artifacts", id],
    queryFn: () => runs.listTaskArtifacts(id),
    retry: false,
  });

  const stopMut = useMutation({
    mutationFn: () => tasks.stopTask(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["task", id] });
      toast("Run stopped", "stop");
    },
    onError: (e: unknown) => toast(e instanceof Error ? e.message : "Couldn't stop run", "alert"),
  });

  const sendPaused = useMutation({
    mutationFn: (body: string) => feedback.addFeedback(id, body),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["feedback", id] });
      toast("Feedback sent · agent resuming");
      setPaused(false);
      setPausedBody("");
    },
    onError: (e: unknown) => toast(e instanceof Error ? e.message : "Couldn't send feedback", "alert"),
  });

  const task = taskQuery.data;
  const artifacts: Artifact[] = artifactsQuery.data ?? [];

  const submitPaused = () => {
    const text = pausedBody.trim();
    if (!text) {
      toast("Describe your feedback first", "alert");
      return;
    }
    sendPaused.mutate(text);
  };

  return (
    <>
      <div className="crumbs">
        <Link to="/board">Kanban</Link>
        <span className="sep">/</span>
        <span>In progress</span>
        <span className="sep">/</span>
        <span className="fg2">{shortId(id)}</span>
      </div>

      <AsyncBoundary
        isLoading={taskQuery.isLoading}
        isError={taskQuery.isError}
        error={taskQuery.error}
        data={task}
        emptyTitle="Task not found"
        emptyHint="This task doesn't exist or the backend hasn't implemented it yet."
      >
        {(t) => (
          <TaskHeader
            task={t}
            paused={paused}
            onPause={() => setPaused(true)}
            onStop={() => stopMut.mutate()}
            onReview={() => setReviewOpen(true)}
          />
        )}
      </AsyncBoundary>

      {taskQuery.isLoading && (
        <div className="card" style={{ marginBottom: "var(--space-5)" }}>
          <div className="muted">Loading task…</div>
        </div>
      )}

      {/* Paused inline feedback composer */}
      {paused && (
        <section
          className="card mt-6"
          style={{
            marginBottom: "var(--space-5)",
            border: "1px solid color-mix(in oklab, var(--warn) 50%, transparent)",
          }}
        >
          <div className="spread" style={{ alignItems: "flex-start", flexWrap: "wrap", gap: "var(--space-4)" }}>
            <div>
              <span style={{ display: "inline-block", marginBottom: 8 }}>
                <Badge tone="warn" dot>
                  paused · awaiting your feedback
                </Badge>
              </span>
              <h2 style={{ fontSize: 22, marginTop: 4 }}>Agent is paused and waiting</h2>
              <p className="muted" style={{ fontSize: 14, maxWidth: 560, marginTop: 4 }}>
                Write the feedback the agent should apply before continuing. This is the substantive guidance that
                steers the run — not a passing note.
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setPaused(false);
                setPausedBody("");
              }}
            >
              Cancel pause
            </Button>
          </div>

          <hr style={{ border: "none", borderTop: "1px solid var(--border-soft)", margin: "var(--space-4) 0" }} />

          <Field label="Your feedback" help="Be specific — the agent will break it down and apply it.">
            <Textarea
              placeholder="e.g. parseClaims should return a clear error when the token is expired — don't silently return a zero value."
              value={pausedBody}
              onChange={(e) => setPausedBody((e.target as HTMLTextAreaElement).value)}
            />
          </Field>

          <div className="row" style={{ justifyContent: "flex-end", marginTop: "var(--space-4)" }}>
            <Button
              variant="primary"
              onClick={submitPaused}
              disabled={sendPaused.isPending}
              icon={<Icon name="send" size={15} />}
            >
              Send feedback &amp; resume
            </Button>
          </div>
        </section>
      )}

      <Tabs
        tabs={[
          {
            key: "output",
            label: "Output",
            content: (
              <div className="grid" style={{ gridTemplateColumns: "1fr 300px" }}>
                <section className="card flush" style={{ height: "fit-content" }}>
                  <div
                    className="spread"
                    style={{
                      padding: "var(--space-3) var(--space-4)",
                      borderBottom: "1px solid var(--border-soft)",
                    }}
                  >
                    <div className="row" style={{ alignItems: "center", gap: 8 }}>
                      {connected ? (
                        <Badge tone="accent" dot pulse>
                          streaming
                        </Badge>
                      ) : (
                        <Badge tone="muted" dot>
                          offline
                        </Badge>
                      )}
                      <span className="mono muted" style={{ fontSize: 12 }}>
                        forge · /tasks/{shortId(id)}
                      </span>
                    </div>
                  </div>
                  <Terminal steps={steps} connected={connected} />
                </section>

                <div className="stack">
                  <StepsList steps={steps} />
                </div>
              </div>
            ),
          },
          {
            key: "artifacts",
            label: `Artifacts (${artifacts.length})`,
            content: (
              <AsyncBoundary
                isLoading={artifactsQuery.isLoading}
                isError={artifactsQuery.isError}
                error={artifactsQuery.error}
                data={artifacts}
                isEmpty={(d) => d.length === 0}
                emptyTitle="No artifacts yet"
                emptyHint="Patches and documents produced by the run appear here."
              >
                {(data) => (
                  <div className="grid" style={{ gridTemplateColumns: "repeat(2, 1fr)" }}>
                    {data.map((a) => (
                      <Card key={a.id}>
                        <div className="spread">
                          <div className="row" style={{ alignItems: "center", gap: 12 }}>
                            <span
                              className="feed-dot"
                              style={{ background: "var(--surface)", width: 30, height: 30 }}
                            >
                              <Icon name={a.kind === "patch" ? "code" : "file"} size={16} />
                            </span>
                            <div>
                              <div style={{ fontWeight: 600 }}>{a.filename}</div>
                              <div className="mono muted" style={{ fontSize: 12 }}>
                                {a.kind === "patch" && a.additions != null && a.deletions != null
                                  ? `+${a.additions} / −${a.deletions} · patch`
                                  : a.kind === "patch"
                                    ? "patch"
                                    : a.summary || "document"}
                              </div>
                            </div>
                          </div>
                          <Button variant="soft" size="sm">
                            {a.kind === "patch" ? "View diff" : "Open"}
                          </Button>
                        </div>
                      </Card>
                    ))}
                  </div>
                )}
              </AsyncBoundary>
            ),
          },
          {
            key: "feedback",
            label: "Feedback log",
            content: <FeedbackThread taskId={id} />,
          },
        ]}
      />

      <ReviewModal open={reviewOpen} onClose={() => setReviewOpen(false)} taskId={id} />
    </>
  );
}
