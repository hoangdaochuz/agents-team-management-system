#!/usr/bin/env bash
# Refresh the digest pins in every Dockerfile base image. Digests make builds
# reproducible and deployable images immutable; tags alone can be repushed.
#
# Usage: ./deploy/pin-digests.sh   (or `make pin-digests`)
# Requires: curl, python3. Adds/updates `name:tag@sha256:...` references.
set -euo pipefail

# image tag -> the Dockerfiles it may appear in (relative to repo root)
declare -A IMAGES=(
  ["golang:1.25-alpine"]="deploy/service.Dockerfile deploy/runner.Dockerfile"
  ["golang:1.22-bookworm"]="backend/runner/Dockerfile"
  ["alpine:3.20"]="deploy/runner.Dockerfile"
  ["node:20-alpine"]="frontend/Dockerfile"
  ["nginxinc/nginx-unprivileged:1.27-alpine"]="frontend/Dockerfile"
)
# Single-registry (non-hub) references handled separately below.
DISTROLESS_STATIC_NONROOT="gcr.io/distroless/static-debian12:nonroot"
DISTROLESS_FILES="deploy/service.Dockerfile"

cd "$(git rev-parse --show-toplevel)"

pin() { # pin <file> <from-line-prefix> <new-reference>
  local file="$1" prefix="$2" ref="$3"
  python3 - "$file" "$prefix" "$ref" <<'EOF'
import re, sys
path, prefix, ref = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path) as f:
    lines = f.readlines()
out, changed = [], False
for line in lines:
    if line.startswith(f"FROM {prefix}") and line.rstrip() != f"FROM {ref}":
        alias = ""
        m = re.match(r"FROM \S+\s+AS\s+(\S+)", line)
        if m:
            alias = f" AS {m.group(1)}"
        line = f"FROM {ref}{alias}\n"
        changed = True
    out.append(line)
if changed:
    with open(path, "w") as f:
        f.writelines(out)
EOF
}

hub_digest() { # hub_digest <repo>:<tag>  (official images get the library/ ns)
  local img="$1" repo="${1%:*}" attempt digest=""
  case "$repo" in */*) ;; *) repo="library/$repo" ;; esac
  for attempt in 1 2 3; do
    digest="$(curl -sf "https://hub.docker.com/v2/repositories/$repo/tags/${img#*:}" \
      | python3 -c 'import json,sys; print(json.load(sys.stdin)["digest"])' 2>/dev/null)" && break
    sleep $((attempt * 2)) # registry rate-limit blips are common; back off
  done
  [ -n "$digest" ] || { echo "FAILED to resolve $img" >&2; exit 1; }
  echo "$digest"
}

gcr_digest() { # gcr_digest <full-image>:<tag>  (registry path drops the host)
  local img="$1" attempt digest=""
  local repo="${img%:*}" # drop :tag first…
  repo="${repo#*/}"      # …then the "gcr.io/" host prefix
  for attempt in 1 2 3 4 5; do
    # GET with a header dump (HEAD gets rate-limited in bursts on gcr.io).
    digest="$(curl -s -o /dev/null -D - "https://gcr.io/v2/$repo/manifests/${img##*:}" \
      -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json" \
      | tr -d '\r' | awk 'tolower($1)=="docker-content-digest:"{print $2}')" || true
    [ -n "$digest" ] && break
    sleep $((attempt * 3))
  done
  [ -n "$digest" ] || { echo "FAILED to resolve $img" >&2; exit 1; }
  echo "$digest"
}

for img in "${!IMAGES[@]}"; do
  digest="$(hub_digest "$img")"
  echo "$img -> ${digest:0:19}..."
  for file in ${IMAGES[$img]}; do
    pin "$file" "$img" "$img@$digest"
  done
done

digest="$(gcr_digest "$DISTROLESS_STATIC_NONROOT")"
echo "$DISTROLESS_STATIC_NONROOT -> ${digest:0:19}..."
for file in $DISTROLESS_FILES; do
  pin "$file" "$DISTROLESS_STATIC_NONROOT" "$DISTROLESS_STATIC_NONROOT@$digest"
done

echo "Done. Review with: git diff -- '*Dockerfile*'"
