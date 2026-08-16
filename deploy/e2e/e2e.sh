#!/usr/bin/env bash
# E2E for the event-driven microservices backend (change tasks 9.2/9.3/14.1/14.2).
#
# Requires: `docker compose up -d` (deploy/docker-compose.yml), curl, jq.
# Usage:    ./deploy/e2e/e2e.sh [GATEWAY_URL]   (default http://localhost:8080)
#
# Covers:
#   9.3  CRUD shapes, SSE replay+tail, review-round loop, stop aborts,
#        PR created on demand (never auto-merged)
#   14.2 401/403 enforcement, cross-workspace isolation on unscoped lists
#
# The Runner runs with RUNNER_DRIVER=simulated, whose reviewer returns
# REQUEST_CHANGES on round 1 and APPROVE on round >= 2 — so a task lands on
# `done` after exactly two run cycles. Deterministic by design.
set -u

GATEWAY="${1:-http://localhost:8080}"
API="$GATEWAY/api"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
DCOMPOSE="${DCOMPOSE:-docker compose -f deploy/docker-compose.yml}"

PASS=0
FAIL=0

jq_ok() { command -v jq >/dev/null || { echo "e2e: jq is required"; exit 2; }; }

check() { # name expected actual
  if [ "$2" = "$3" ]; then
    PASS=$((PASS + 1)); echo "  ok: $1"
  else
    FAIL=$((FAIL + 1)); echo "  FAIL: $1 (want '$2' got '$3')"
  fi
}

# http_code <jar> <method> <path> [body] -> prints HTTP status
http_code() {
  local jar="$1" method="$2" path="$3" body="${4:-}"
  if [ -n "$body" ]; then
    curl -s -o /dev/null -w '%{http_code}' -b "$jar" -c "$jar" -X "$method" \
      -H 'Content-Type: application/json' -d "$body" "$API$path"
  else
    curl -s -o /dev/null -w '%{http_code}' -b "$jar" -c "$jar" -X "$method" "$API$path"
  fi
}

# json <jar> <method> <path> [body] -> prints response body
json() {
  local jar="$1" method="$2" path="$3" body="${4:-}"
  if [ -n "$body" ]; then
    curl -s -b "$jar" -c "$jar" -X "$method" -H 'Content-Type: application/json' \
      -d "$body" "$API$path"
  else
    curl -s -b "$jar" -c "$jar" -X "$method" "$API$path"
  fi
}

# poll <name> <max_seconds> <expr...> -> runs eval "$*" in THIS shell (the
# json() helper is a function, not a binary — bash -c would lose it) until 0.
poll() {
  local name="$1" max="$2"; shift 2
  local i=0
  while ! eval "$*" >/dev/null 2>&1; do
    i=$((i + 1))
    if [ "$i" -ge "$max" ]; then echo "  FAIL: $name (timeout)"; FAIL=$((FAIL + 1)); return 1; fi
    sleep 1
  done
  echo "  ok: $name"
}

jq_ok

echo "== gateway health =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$GATEWAY/healthz")
check "gateway /healthz up" "200" "$code"

# Idempotency: wipe the test users from previous runs so re-runs work.
EM="'alice@aaks.dev','bob@aaks.dev','carol@aaks.dev'"
UIDs=$($DCOMPOSE exec -T postgres psql -U aaks -d auth_db -tAc "SELECT string_agg(id::text, ',') FROM users WHERE email IN ($EM)")
if [ -n "$UIDs" ]; then
  $DCOMPOSE exec -T postgres psql -U aaks -d orgs_db -v ON_ERROR_STOP=1 >/dev/null 2>&1 <<SQL || true
DELETE FROM join_requests WHERE email IN ($EM);
DELETE FROM org_requests WHERE email IN ($EM);
DELETE FROM invites WHERE email IN ($EM);
DELETE FROM invites WHERE workspace_id IN (SELECT id FROM workspaces WHERE organization_id IN (SELECT id FROM organizations WHERE owner_id IN ($UIDs)));
DELETE FROM workspaces WHERE organization_id IN (SELECT id FROM organizations WHERE owner_id IN ($UIDs));
DELETE FROM memberships WHERE user_email IN ($EM) OR user_id IN ($UIDs);
DELETE FROM organizations WHERE owner_id IN ($UIDs);
SQL
fi
$DCOMPOSE exec -T postgres psql -U aaks -d auth_db -v ON_ERROR_STOP=1 >/dev/null 2>&1 <<SQL || true
DELETE FROM sessions WHERE user_id IN (SELECT id FROM users WHERE email IN ($EM));
DELETE FROM signup_requests WHERE email IN ($EM);
DELETE FROM invite_codes WHERE email IN ($EM);
DELETE FROM users WHERE email IN ($EM);
SQL

JADMIN="$TMP/admin.jar"
J1="$TMP/user1.jar"
J2="$TMP/user2.jar"

echo "== signup: create mode + sysadmin approval (14.1) =="
code=$(http_code "$J1" POST /auth/signup \
  '{"full_name":"Alice","email":"alice@aaks.dev","password":"password123","start_mode":"create","organization_name":"Acme Inc"}')
check "alice signup accepted" "201" "$code"

code=$(http_code "$J2" POST /auth/signup \
  '{"full_name":"Bob","email":"bob@aaks.dev","password":"password123","start_mode":"create","organization_name":"Globex"}')
check "bob signup accepted" "201" "$code"

state=$(json "$J1" GET /auth/signup-status | jq -r '.state')
check "alice signup pending" "pending" "$state"

code=$(http_code "$JADMIN" POST /auth/login '{"email":"admin@aaks.dev","password":"adminpass123"}')
check "superadmin login" "200" "$code"

# Unapproved users cannot log in (10.3).
code=$(http_code "$J1" POST /auth/login '{"email":"alice@aaks.dev","password":"password123"}')
check "alice login blocked before approval" "403" "$code"

for email in alice@aaks.dev bob@aaks.dev; do
  rid=$(json "$JADMIN" GET /sysadmin/requests | jq -r --arg e "$email" '.[] | select(.email==$e) | .id' | head -1)
  code=$(http_code "$JADMIN" POST "/sysadmin/requests/$rid/approve" '{}')
  check "approve $email" "204" "$code"
done

code=$(http_code "$J1" POST /auth/login '{"email":"alice@aaks.dev","password":"password123"}')
check "alice login after approval" "200" "$code"
code=$(http_code "$J2" POST /auth/login '{"email":"bob@aaks.dev","password":"password123"}')
check "bob login after approval" "200" "$code"

WS1=$(json "$J1" GET /workspaces | jq -r '.[0].id')
WS2=$(json "$J2" GET /workspaces | jq -r '.[0].id')
check "alice has a workspace" "1" "$(json "$J1" GET /workspaces | jq length)"
check "bob has a workspace" "1" "$(json "$J2" GET /workspaces | jq length)"
check "workspaces differ (isolation)" "1" "$([ "$WS1" != "$WS2" ] && echo 1 || echo 0)"

echo "== 401 / 403 enforcement (14.2) =="
code=$(curl -s -o /dev/null -w '%{http_code}' "$API/tasks")
check "no-session GET /tasks -> 401" "401" "$code"
code=$(curl -s -o /dev/null -w '%{http_code}' "$API/workspaces")
check "no-session GET /workspaces -> 401" "401" "$code"
code=$(http_code "$J1" GET /sysadmin/flags)
check "non-superadmin GET /sysadmin/flags -> 403" "403" "$code"
code=$(http_code "$JADMIN" GET /sysadmin/flags)
check "superadmin GET /sysadmin/flags" "200" "$code"
code=$(http_code "$JADMIN" GET /sysadmin/kpis)
check "superadmin GET /sysadmin/kpis" "200" "$code"

echo "== task lifecycle through the saga (9.3) =="
code=$(http_code "$J1" POST /agents \
  '{"name":"Alice Implementer","role":"implementer","default_model":"simulated/sim","system_prompt":"You implement tasks."}')
check "alice creates agent" "201" "$code"
AGENT1=$(json "$J1" GET /agents | jq -r '.[0].id')
code=$(http_code "$J1" POST /projects \
  '{"name":"alice-repo","repo_source":"https://github.com/example/alice-repo.git","repo_type":"git"}')
check "alice creates project" "201" "$code"
PROJ1=$(json "$J1" GET /projects | jq -r '.[0].id')
code=$(http_code "$J1" POST /tasks \
  "{\"project_id\":\"$PROJ1\",\"agent_id\":\"$AGENT1\",\"title\":\"Add login page\",\"prompt\":\"Implement the login page.\"}")
check "alice creates task" "201" "$code"
T1=$(json "$J1" GET /tasks | jq -r '.[0].id')

# Cross-workspace isolation on the unscoped list (14.2).
code=$(json "$J2" GET /tasks | jq -r --arg t "$T1" 'any(.id == $t)')
check "bob does not see alice's task" "false" "$code"

code=$(http_code "$J1" PATCH "/tasks/$T1/status" '{"status":"doing"}')
check "alice moves task to doing" "200" "$code"

poll "task reaches done after review rounds" 90 'json "$J1" GET "/tasks/$T1" | jq -r .status | grep -qx done'
status=$(json "$J1" GET "/tasks/$T1" | jq -r .status)
check "task final status is done" "done" "$status"
runs=$(json "$J1" GET "/tasks/$T1/runs" | jq length)
check "runs: implementer+reviewer x2" "4" "$runs"

echo "== SSE replay + tail (9.3) =="
steps=$(curl -s -N --max-time 10 -b "$J1" "$API/tasks/$T1/stream" | grep -c '^event: step')
check "SSE stream emits step events" "1" "$([ "$steps" -ge 1 ] && echo 1 || echo 0)"

echo "== PR created on demand, never auto-merged (9.3) =="
code=$(http_code "$J1" POST "/tasks/$T1/open-pr" '{}')
check "open-pr accepted" "202" "$code"
status=$(json "$J1" GET "/tasks/$T1" | jq -r .status)
check "task still done after PR (no status change)" "done" "$status"

echo "== stop aborts (9.3) =="
code=$(http_code "$J2" POST /agents \
  '{"name":"Bob Implementer","role":"implementer","default_model":"simulated/sim","system_prompt":"You implement tasks."}')
AGENT2=$(json "$J2" GET /agents | jq -r '.[0].id')
code=$(http_code "$J2" POST /projects \
  '{"name":"bob-repo","repo_source":"https://github.com/example/bob-repo.git","repo_type":"git"}')
PROJ2=$(json "$J2" GET /projects | jq -r '.[0].id')
code=$(http_code "$J2" POST /tasks \
  "{\"project_id\":\"$PROJ2\",\"agent_id\":\"$AGENT2\",\"title\":\"Long task\",\"prompt\":\"Do a long job.\"}")
T2=$(json "$J2" GET /tasks | jq -r '.[0].id')
code=$(http_code "$J2" PATCH "/tasks/$T2/status" '{"status":"doing"}')
check "bob moves task to doing" "200" "$code"
code=$(http_code "$J2" POST "/tasks/$T2/stop" '{}')
check "stop accepted" "200" "$code"
poll "task stops" 20 'json "$J2" GET "/tasks/$T2" | jq -r .status | grep -qx stopped'
status=$(json "$J2" GET "/tasks/$T2" | jq -r .status)
check "task status is stopped" "stopped" "$status"

echo "== join mode: invite -> signup -> workspace-admin approval (14.1) =="
CODE=$(http_code "$J1" POST "/workspaces/$WS1/invites" '{"emails":["carol@aaks.dev"],"role":"member"}')
check "alice invites carol" "201" "$CODE"
# The invite code is delivered by email in production; read it from the DB.
INVITE=$($DCOMPOSE exec -T postgres \
  psql -U aaks -d orgs_db -tAc "SELECT invite_code FROM invites WHERE email='carol@aaks.dev' ORDER BY created_at DESC LIMIT 1" | tr -d '[:space:]')
check "invite code persisted" "1" "$([ -n "$INVITE" ] && echo 1 || echo 0)"
JC="$TMP/carol.jar"
code=$(http_code "$JC" POST /auth/signup \
  "{\"full_name\":\"Carol\",\"email\":\"carol@aaks.dev\",\"password\":\"password123\",\"start_mode\":\"join\",\"invite_code\":\"$INVITE\"}")
check "carol join signup accepted" "201" "$code"
rid=$(json "$J1" GET "/workspaces/$WS1/requests" | jq -r --arg e "carol@aaks.dev" '.[] | select(.email==$e) | .id' | head -1)
code=$(http_code "$J1" POST "/workspaces/$WS1/requests/$rid/approve" '{}')
check "alice approves carol's join" "204" "$code"
code=$(http_code "$JC" POST /auth/login '{"email":"carol@aaks.dev","password":"password123"}')
check "carol login after join approval" "200" "$code"
code=$(json "$JC" GET /tasks | jq -r --arg t "$T1" 'any(.id == $t)')
check "carol (ws1 member) sees alice's task" "true" "$code"

echo
echo "E2E result: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
