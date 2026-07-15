# Base image for per-task execution containers (the credential-less build/test
# sandbox the backend drives via the Docker exec API).
#
# SECURITY INVARIANT: this image and its runtime MUST hold NO API keys and NO
# git credentials. The task worktree is bind-mounted at /workspace at runtime.
# Do not add secrets, tokens, or keys here.

FROM golang:1.22-bookworm

# Broad toolchain so agent run_command (build/test/lint) works for typical repos.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      git ca-certificates curl make gcc \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
# Default: keep the container alive so the backend can exec into it.
CMD ["sleep", "infinity"]
