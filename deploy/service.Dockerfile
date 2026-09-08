# Multi-stage build for an individual service binary. Arg SERVICE selects the
# target (gateway, identity, workspace, agent); ARG PORT exposes the service
# port. (executor uses deploy/runner.Dockerfile — it needs git at runtime.)
# syntax=docker/dockerfile:1

# ---- build ----
# Base images pinned by digest (see `make pin-digests` to refresh): immutable,
# reproducible builds — a mutable tag can be repushed under our feet.
FROM golang:1.25-alpine@sha256:1ae0735f00daffa3aaf1363a5184c0d2dc55c78e3db4ec70241cdac97bf84b59 AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./services/${SERVICE}/cmd

# ---- runtime ----
# distroless nonroot: minimal surface, no shell. Secrets are injected at runtime
# via env; nothing is baked into the image (credential-less sandbox invariant).
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/service /service
ARG PORT=8080
EXPOSE ${PORT}
USER nonroot:nonroot
ENTRYPOINT ["/service"]
