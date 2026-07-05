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
# Shared configuration and helpers for AX Cloud Run automation scripts.
# Source this file: `source "$(dirname "$0")/lib.sh"`.

set -euo pipefail

# --- Configuration (override via environment) --------------------------------

# GCP project and Cloud Run region.
PROJECT="${AX_PROJECT:-${GOOGLE_CLOUD_PROJECT:-kunal-scratch}}"
REGION="${AX_REGION:-us-central1}"

# Artifact Registry repo and image for the ADK weather agent.
AR_REPO="${AX_AR_REPO:-ax}"
IMAGE_NAME="${AX_IMAGE_NAME:-adk-weather-agent}"
IMAGE_TAG="${AX_IMAGE_TAG:-latest}"

# Cloud Run service backing the ADK agent template.
SERVICE="${AX_SERVICE:-ax-adk-weather}"

# Vertex / Gemini configuration passed to the agent container.
ADK_MODEL="${AX_ADK_MODEL:-gemini-2.5-flash}"
VERTEX_LOCATION="${AX_VERTEX_LOCATION:-$REGION}"

# Auth mode for the Cloud Run service. User (ADC) principals cannot mint
# audience-scoped ID tokens, so the default verification flow deploys the service
# as allow-unauthenticated. Set AX_ALLOW_UNAUTH=0 to require IAM ID-token auth
# (recommended in production with a service account principal).
ALLOW_UNAUTH="${AX_ALLOW_UNAUTH:-1}"

# Repository root (this file lives in scripts/).
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Derived values.
AR_HOST="${REGION}-docker.pkg.dev"
IMAGE_URI="${AR_HOST}/${PROJECT}/${AR_REPO}/${IMAGE_NAME}:${IMAGE_TAG}"

# --- Helpers -----------------------------------------------------------------

log()  { printf '\033[1;34m[ax]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[1;33m[ax]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m[ax]\033[0m %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

# ensure_auth verifies gcloud is authenticated and a project is resolvable.
ensure_auth() {
  require_cmd gcloud
  local acct
  acct="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | head -1)"
  [ -n "$acct" ] || die "no active gcloud account; run 'gcloud auth login' and 'gcloud auth application-default login'"
  log "using account: $acct  project: $PROJECT  region: $REGION"
}
