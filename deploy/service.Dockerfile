# Multi-stage build for an individual service binary. Arg SERVICE selects the
# target (gateway, project, task, agent, catalog, settings, runner, auth,
# orgs, resources, admin); ARG PORT exposes the service port.
# syntax=docker/dockerfile:1

# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/service ./services/${SERVICE}/cmd

# ---- runtime ----
# distroless nonroot: minimal surface, no shell. Secrets are injected at runtime
# via env; nothing is baked into the image (credential-less sandbox invariant).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/service /service
ARG PORT=8080
EXPOSE ${PORT}
USER nonroot:nonroot
ENTRYPOINT ["/service"]
