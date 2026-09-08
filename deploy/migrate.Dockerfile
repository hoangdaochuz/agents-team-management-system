# Migration runner image — upstream golang-migrate, digest-pinned for
# reproducibility, repushed to our Artifact Registry by CI so the Kyverno
# verify-images policy (Artifact Registry origin only) admits the migration
# PreSync Jobs. Migrations themselves ride in a ConfigMap populated by CI
# from backend/services/<svc>/internal/infrastructure/repository/migrations.
# syntax=docker/dockerfile:1

# Upstream binary, pinned by manifest-list digest (migrate/migrate v4.17.1).
FROM migrate/migrate:v4.17.1@sha256:de154de4b7f9d0d751aacb1ec6023f5fc96f874eb88b65d050f761a33376aa4b
# No extra layers: the image already runs `migrate` as its entrypoint.
