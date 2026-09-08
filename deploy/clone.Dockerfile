# Clone-bootstrap image — git + ca-certificates on the same pinned alpine
# as the executor runtime. Runs the clone-bootstrap CronJob that seeds the
# Filestore clone root (the backend never clones; see design D3).
#
# SECURITY: public toolchain only — no credentials baked in. Repo access uses
# a token mounted at runtime, never an image layer.
# syntax=docker/dockerfile:1

FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
RUN apk add --no-cache git ca-certificates openssh-client \
    && addgroup -g 1000 cloner \
    && adduser -D -u 1000 -G cloner cloner
USER cloner:cloner
ENTRYPOINT ["git"]
