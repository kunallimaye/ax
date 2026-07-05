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

package runtime

import "context"

// Compile-time interface assertion.
var _ Runtime = (*LocalRuntime)(nil)

// LocalRuntime is the trivial substrate: the agent's HarnessService is assumed
// to already be listening at a fixed local address (e.g. a sidecar process). It
// performs no provisioning; Activate simply returns the configured address.
type LocalRuntime struct {
	address string
}

// NewLocalRuntime creates a LocalRuntime pointing at addr. If addr is empty it
// defaults to "127.0.0.1:50053".
func NewLocalRuntime(addr string) *LocalRuntime {
	if addr == "" {
		addr = "127.0.0.1:50053"
	}
	return &LocalRuntime{address: addr}
}

// Name implements Runtime.
func (r *LocalRuntime) Name() string { return "local" }

// Activate returns the fixed local endpoint (insecure, no audience).
func (r *LocalRuntime) Activate(ctx context.Context, conversationID string) (*Endpoint, error) {
	return &Endpoint{Address: r.address, UseTLS: false}, nil
}

// Deactivate is a no-op for the local runtime.
func (r *LocalRuntime) Deactivate(ctx context.Context, conversationID string) error { return nil }

// Teardown is a no-op for the local runtime.
func (r *LocalRuntime) Teardown(ctx context.Context, conversationID string) error { return nil }

// Close is a no-op for the local runtime.
func (r *LocalRuntime) Close() error { return nil }
