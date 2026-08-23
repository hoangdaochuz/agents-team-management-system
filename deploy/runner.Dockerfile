# Runner service image — the ONLY service image that needs a runtime with
# `git` (the runner creates/removes per-task worktrees host-side via
# `git worktree`, and the clone root is shared with its DinD daemon).
# Everyone else keeps the distroless service.Dockerfile.
#
# SECURITY: the runner talks to Settings over the internal token path for
# provider keys; nothing sensitive is baked into this image.
# syntax=docker/dockerfile:1

# ---- build (same as service.Dockerfile) ----
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./services/runner/cmd

# ---- runtime: alpine + git + ca-certificates, non-root ----
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
RUN apk add --no-cache git ca-certificates tzdata \
    && addgroup -g 1000 runner \
    && adduser -D -u 1000 -G runner runner
COPY --from=build /out/service /service
USER runner:runner
EXPOSE 8086
ENTRYPOINT ["/service"]
