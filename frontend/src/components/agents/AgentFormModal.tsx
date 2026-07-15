import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { agents } from "../../api/client";
import type { Agent } from "../../api/types";
import { Field, Input, Select, Textarea, Button, Modal, useToast } from "../ui";

const MODELS = ["gpt-4o", "gpt-4o-mini", "claude-sonnet-4", "claude-haiku-4", "gemini-1.5-pro", "glm-4.5"];

export function AgentFormModal({
  open,
  onClose,
  agent,
}: {
  open: boolean;
  onClose: () => void;
  agent?: Agent;
}) {
  const qc = useQueryClient();
  const { toast } = useToast();
  const editing = Boolean(agent);

  const [name, setName] = useState("");
  const [role, setRole] = useState("");
  const [defaultModel, setDefaultModel] = useState(MODELS[0]);
  const [systemPrompt, setSystemPrompt] = useState("");
  const [allowedTools, setAllowedTools] = useState("");

  useEffect(() => {
    if (open) {
      setName(agent?.name ?? "");
      setRole(agent?.role ?? "");
      setDefaultModel(agent?.default_model ?? MODELS[0]);
      setSystemPrompt(agent?.system_prompt ?? "");
      setAllowedTools((agent?.allowed_tools ?? []).join(", "));
    }
  }, [open, agent]);

  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        name: name.trim(),
        role: role.trim(),
        default_model: defaultModel,
        system_prompt: systemPrompt.trim(),
        allowed_tools: allowedTools
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
      };
      if (editing && agent) {
        return agents.updateAgent(agent.id, payload);
      }
      return agents.createAgent(payload);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["agents"] });
      toast(editing ? "Agent updated" : "Agent created");
      onClose();
    },
    onError: (e: unknown) => {
      const msg = e instanceof Error ? e.message : "Failed to save agent";
      toast(msg, "alert");
    },
  });

  const submit = () => {
    if (!name.trim() || !role.trim()) {
      toast("Name and role are required", "alert");
      return;
    }
    mutation.mutate();
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? "Edit agent" : "New agent"}
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={submit} disabled={mutation.isPending}>
            {mutation.isPending ? "Saving…" : editing ? "Save changes" : "Create agent"}
          </Button>
        </>
      }
    >
      <Field label="Agent name">
        <Input
          placeholder="e.g. Pixel — UI engineer"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>

      <Field label="Role">
        <Input
          placeholder="e.g. Frontend / React"
          value={role}
          onChange={(e) => setRole(e.target.value)}
        />
      </Field>

      <Field label="Default model">
        <Select value={defaultModel} onChange={(e) => setDefaultModel(e.target.value)}>
          {MODELS.map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>
      </Field>

      <Field label="System prompt">
        <Textarea
          rows={4}
          placeholder="You are a senior engineer. Follow the project style guide…"
          value={systemPrompt}
          onChange={(e) => setSystemPrompt(e.target.value)}
        />
      </Field>

      <Field label="Allowed tools" help="Comma-separated, e.g. shell, edit, read, web_search">
        <Input
          placeholder="shell, edit, read"
          value={allowedTools}
          onChange={(e) => setAllowedTools(e.target.value)}
        />
      </Field>
    </Modal>
  );
}
