#!/usr/bin/env bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# End-to-end "run on Cloud Run": ensures the ADK weather agent is deployed as a
# Cloud Run service, then executes a single AX turn against it using the cloudrun
# runtime. Invoked by `make run-on-run PROMPT="..."`.
#
# Usage:
#   scripts/run-on-run.sh "what is the weather in London, England?"
#   PROMPT="..." scripts/run-on-run.sh
#
# Environment overrides: see scripts/lib.sh (AX_PROJECT, AX_REGION, AX_SERVICE...).
# Set AX_SKIP_DEPLOY=1 to skip the build/deploy step and reuse an existing service.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

PROMPT="${1:-${PROMPT:-}}"
[ -n "$PROMPT" ] || die "no prompt provided; pass an argument or set PROMPT=..."

ensure_auth
require_cmd go

# 1. Ensure the Cloud Run service exists (build + deploy unless skipped).
if [ "${AX_SKIP_DEPLOY:-0}" = "1" ]; then
  log "AX_SKIP_DEPLOY=1: skipping build/deploy, reusing service '${SERVICE}'"
  URL="$(gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" --format='value(status.url)' 2>/dev/null || true)"
  [ -n "$URL" ] || die "service '${SERVICE}' not found; deploy first or unset AX_SKIP_DEPLOY"
else
  log "deploying ADK weather agent to Cloud Run (this can take a few minutes on first run)"
  URL="$("$(dirname "${BASH_SOURCE[0]}")/deploy-adk-cloudrun.sh" | tail -1)"
  [ -n "$URL" ] || die "deploy did not return a service URL"
fi
log "cloud run service URL: ${URL}"

# 2. Generate an AX config that targets the cloudrun runtime + adk harness.
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
mkdir -p "$WORKDIR/eventlog"
CONFIG="$WORKDIR/ax.yaml"
cat > "$CONFIG" <<EOF
version: v1alpha
server:
  address: ":8494"
eventlog:
  sqlite:
    filename: "$WORKDIR/eventlog/log.sqlite"
runtime:
  default: cloudrun
  cloudrun:
    project: "$PROJECT"
    region: "$REGION"
    service: "$SERVICE"
    allowUnauthenticated: $([ "$ALLOW_UNAUTH" = "1" ] && echo true || echo false)
harnesses:
  adk:
    enabled: true
    default: true
EOF
log "generated config: $CONFIG"

# 3. Build the ax binary and run a single headless turn.
BIN="$WORKDIR/ax"
log "building ax binary"
( cd "$REPO_ROOT" && go build -o "$BIN" ./cmd/ax ) || die "failed to build ax"

log "executing: ax exec --once --input \"$PROMPT\""
"$BIN" exec --once --config "$CONFIG" --input "$PROMPT"
