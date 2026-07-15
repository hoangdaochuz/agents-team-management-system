import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { tasks, agents } from "../../api/client";
import type { TaskType, Priority } from "../../api/types";
import { Field, Input, Select, Textarea, Button, Modal, useToast } from "../ui";

type AgentOption = { id: string; name: string; role: string };

export function CreateTaskModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const qc = useQueryClient();
  const { toast } = useToast();

  // Hardcode the demo project id; the real value comes from project context once
  // the projects endpoint exists. The createTask contract requires a project_id.
  const PROJECT_ID = "00000000-0000-0000-0000-000000000001";

  const { data: agentList } = useQuery<AgentOption[]>({
    queryKey: ["agents"],
    queryFn: async () => {
      const list = await agents.listAgents();
      return list.map((a) => ({ id: a.id, name: a.name, role: a.role }));
    },
  });

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [type, setType] = useState<TaskType>("task");
  const [priority, setPriority] = useState<Priority>("medium");
  const [labels, setLabels] = useState("");
  const [points, setPoints] = useState("3");
  const [assignee, setAssignee] = useState("");

  useEffect(() => {
    if (open) {
      setTitle("");
      setDescription("");
      setType("task");
      setPriority("medium");
      setLabels("");
      setPoints("3");
      setAssignee("");
    }
  }, [open]);

  const mutation = useMutation({
    mutationFn: () =>
      tasks.createTask({
        project_id: PROJECT_ID,
        title: title.trim(),
        prompt: description.trim() || title.trim(),
        description: description.trim() || undefined,
        type,
        priority,
        labels: labels.trim() ? labels.split(",").map((s) => s.trim()).filter(Boolean) : undefined,
        points: Number(points) || undefined,
        agent_id: assignee || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["tasks"] });
      toast("Issue created and added to Backlog");
      onClose();
    },
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : "Failed to create issue";
      toast(msg, "alert");
    },
  });

  const submit = () => {
    if (!title.trim()) {
      toast("Title is required", "alert");
      return;
    }
    mutation.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Create issue"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} disabled={mutation.isPending}>
            {mutation.isPending ? "Creating…" : "Create"}
          </Button>
        </>
      }
    >
      <div className="row" style={{ gap: "var(--space-4)" }}>
        <Field label="Type" className="flex-1">
          <Select value={type} onChange={(e) => setType(e.target.value as TaskType)}>
            <option value="task">Task</option>
            <option value="story">Story</option>
            <option value="bug">Bug</option>
            <option value="epic">Epic</option>
          </Select>
        </Field>
        <Field label="Priority">
          <Select value={priority} onChange={(e) => setPriority(e.target.value as Priority)}>
            <option value="medium">Medium</option>
            <option value="highest">Highest</option>
            <option value="high">High</option>
            <option value="low">Low</option>
          </Select>
        </Field>
      </div>

      <Field label="Title">
        <Input
          placeholder="e.g. Add pagination to /v2/tasks"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
        />
      </Field>

      <Field label="Description">
        <Textarea
          rows={3}
          placeholder="Expected output, constraints, related files…"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </Field>

      <div className="row" style={{ gap: "var(--space-4)" }}>
        <Field label="Assign to" className="flex-1">
          <Select value={assignee} onChange={(e) => setAssignee(e.target.value)}>
            <option value="">Auto-pick best agent</option>
            {(agentList ?? []).map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} — {a.role}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Labels">
          <Input
            placeholder="backend, api (comma-separated)"
            value={labels}
            onChange={(e) => setLabels(e.target.value)}
          />
        </Field>
        <Field label="Story points">
          <Input
            type="number"
            value={points}
            onChange={(e) => setPoints(e.target.value)}
            style={{ width: 80 }}
          />
        </Field>
      </div>
    </Modal>
  );
}
