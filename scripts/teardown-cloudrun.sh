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
# Removes the Cloud Run service and (optionally) the pushed image, to avoid
# lingering cost. Safe to run when resources do not exist.
#
#   scripts/teardown-cloudrun.sh            # delete the Cloud Run service
#   AX_DELETE_IMAGE=1 scripts/teardown-cloudrun.sh   # also delete the image

source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

ensure_auth

if gcloud run services describe "$SERVICE" --project "$PROJECT" --region "$REGION" >/dev/null 2>&1; then
  log "deleting Cloud Run service '${SERVICE}'"
  gcloud run services delete "$SERVICE" --project "$PROJECT" --region "$REGION" --quiet
else
  log "Cloud Run service '${SERVICE}' not found; nothing to delete"
fi

if [ "${AX_DELETE_IMAGE:-0}" = "1" ]; then
  log "deleting image ${IMAGE_URI}"
  gcloud artifacts docker images delete "$IMAGE_URI" --project "$PROJECT" --quiet 2>/dev/null || \
    warn "could not delete image (may not exist)"
fi

log "teardown complete"
