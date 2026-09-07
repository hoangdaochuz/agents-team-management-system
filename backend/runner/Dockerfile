# Base image for per-task execution containers (the credential-less build/test
# sandbox the backend drives via the Docker exec API).
#
# SECURITY INVARIANT: this image and its runtime MUST hold NO API keys and NO
# git credentials. The task worktree is bind-mounted at /workspace at runtime.
# Do not add secrets, tokens, or keys here.
#
# NOTE on root: this image deliberately has no USER — agent commands
# legitimately need root inside the DISPOSABLE sandbox (apt-get, toolchains).
# Containment comes from elsewhere: the container is short-lived, holds no
# credentials (invariant above), and is spawned by the DinD daemon inside the
# locked-down aaks-sandbox namespace — not via the Kubernetes API, so it never
# escapes the sandbox plane.

# Base image digest-pinned: the sandbox must be reproducible — the runner
# expects this exact image on the sandbox daemon (see `make pin-digests`).
# Go matches the service toolchain (1.25) so agent builds use one stdlib.
FROM golang:1.25-bookworm@sha256:3b4a11519ad929d1e1d261a12cff056f0c85b735253d7d861346b9c6f8b36437

# Broad toolchain so agent run_command (build/test/lint) works for typical repos.
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      git ca-certificates curl make gcc \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /workspace
# Default: keep the container alive so the backend can exec into it.
CMD ["sleep", "infinity"]
