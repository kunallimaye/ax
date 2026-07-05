// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package runtime defines the substrate abstraction that decides WHERE and HOW
// an agent endpoint is brought up and torn down for a conversation.
//
// A runtime is intentionally separate from a harness: a harness describes WHAT
// the agent is (a HarnessService gRPC endpoint), while a runtime describes where
// that endpoint lives and how its lifecycle is managed. This separation lets the
// same agent (e.g. an ADK-Go agent) run unchanged on different substrates such as
// Cloud Run, the Agent Substrate (ATE/Kubernetes), or a local process.
//
// The abstraction is expressed in the richer ATE vocabulary of activate /
// deactivate / teardown. Each runtime maps its native mechanics onto these:
//
//	                 activate                deactivate            teardown
//	ATE/substrate    CreateActor+Resume      SuspendActor          DeleteActor
//	Cloud Run        ensure service + URL    no-op (autoscaling)   delete service
//	local            return fixed address    no-op                 no-op
package runtime

import "context"

// Endpoint is a routable HarnessService gRPC target produced by Activate.
type Endpoint struct {
	// Address is the "host:port" (or "host:443") gRPC dial target.
	Address string

	// UseTLS indicates the endpoint terminates TLS (e.g. Cloud Run). When
	// false, callers dial with insecure transport credentials.
	UseTLS bool

	// Audience, when non-empty, is the audience an identity token must carry to
	// authenticate to this endpoint (e.g. a Cloud Run service base URL). An
	// empty Audience means no per-call identity token is required.
	Audience string
}

// Runtime provisions and manages the lifecycle of agent endpoints on a
// particular substrate. Implementations must be safe for concurrent use.
type Runtime interface {
	// Name returns the stable runtime identifier (e.g. "cloudrun", "substrate",
	// "local"). It is used for config-driven selection.
	Name() string

	// Activate ensures a HarnessService endpoint is running and reachable for
	// the given conversation, and returns a routable endpoint. Activate must be
	// idempotent: repeated calls for the same conversation return an endpoint to
	// the same logical agent.
	Activate(ctx context.Context, conversationID string) (*Endpoint, error)

	// Deactivate signals that the current turn is complete and the endpoint may
	// be released to a standby/idle state. It must not destroy durable state.
	// For substrates that idle automatically (e.g. Cloud Run scale-to-zero) this
	// may be a no-op.
	Deactivate(ctx context.Context, conversationID string) error

	// Teardown permanently removes any resources provisioned for the
	// conversation. It is best-effort and safe to call when nothing exists.
	Teardown(ctx context.Context, conversationID string) error

	// Close releases runtime-level resources (control-plane connections, API
	// clients). It does not affect provisioned conversation endpoints.
	Close() error
}
