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
# Builds the ADK weather agent image, pushes it to Artifact Registry, and deploys
# it as a Cloud Run service. Idempotent: safe to re-run. Prints the service URL.

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ensure_auth
require_cmd gcloud

log "ensuring required APIs are enabled"
gcloud services enable run.googleapis.com artifactregistry.googleapis.com \
  cloudbuild.googleapis.com aiplatform.googleapis.com \
  --project "$PROJECT" >/dev/null

log "ensuring Artifact Registry repo '${AR_REPO}' exists in ${REGION}"
if ! gcloud artifacts repositories describe "$AR_REPO" \
      --location "$REGION" --project "$PROJECT" >/dev/null 2>&1; then
  gcloud artifacts repositories create "$AR_REPO" \
    --repository-format=docker --location "$REGION" \
    --description="AX agent images" --project "$PROJECT" >/dev/null
  log "created Artifact Registry repo '${AR_REPO}'"
fi

# Build with Cloud Build (no local Docker daemon needed) from the repo root so
# the agent module's local replace of github.com/google/ax resolves. A dedicated
# cloudbuild config points at the agent's Dockerfile.
log "building + pushing image via Cloud Build: ${IMAGE_URI}"
gcloud builds submit "$REPO_ROOT" \
  --project "$PROJECT" \
  --region "$REGION" \
  --config "$REPO_ROOT/cicd/cloudbuild.adk.yaml" \
  --substitutions "_IMAGE=${IMAGE_URI}" \
  1>&2 || die "image build failed"

AUTH_FLAG="--no-allow-unauthenticated"
if [ "$ALLOW_UNAUTH" = "1" ]; then
  AUTH_FLAG="--allow-unauthenticated"
  warn "deploying with --allow-unauthenticated (AX_ALLOW_UNAUTH=1); set AX_ALLOW_UNAUTH=0 for IAM ID-token auth"
fi

log "deploying Cloud Run service '${SERVICE}'"
gcloud run deploy "$SERVICE" \
  --project "$PROJECT" \
  --region "$REGION" \
  --image "$IMAGE_URI" \
  --use-http2 \
  $AUTH_FLAG \
  --min-instances 0 \
  --max-instances 4 \
  --cpu 1 --memory 512Mi \
  --port 8080 \
  --set-env-vars "GOOGLE_CLOUD_PROJECT=${PROJECT},GOOGLE_CLOUD_LOCATION=${VERTEX_LOCATION},ADK_MODEL=${ADK_MODEL}" \
  1>&2 || die "cloud run deploy failed"

URL="$(gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" --format='value(status.url)')"
[ -n "$URL" ] || die "could not resolve service URL after deploy"
log "service deployed: ${URL}"

# Grant the active principal permission to invoke the (authenticated) service so
# AX can mint an ID token that is accepted.
ACTIVE_ACCT="$(gcloud auth list --filter=status:ACTIVE --format='value(account)' | head -1)"
if [ -n "$ACTIVE_ACCT" ]; then
  log "granting run.invoker on '${SERVICE}' to ${ACTIVE_ACCT}"
  gcloud run services add-iam-policy-binding "$SERVICE" \
    --project "$PROJECT" --region "$REGION" \
    --member "user:${ACTIVE_ACCT}" --role roles/run.invoker >/dev/null 2>&1 || \
    warn "could not add run.invoker binding (may already exist or principal is not a user)"
fi

# Emit the URL on stdout for callers to capture.
echo "$URL"
