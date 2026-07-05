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

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/google/ax/internal/k8s/ate"
)

// Compile-time interface assertion.
var _ Runtime = (*SubstrateRuntime)(nil)

// SubstrateRuntime provisions agent endpoints as sandboxed actors on the Agent
// Substrate (ATE) control plane, which runs on Kubernetes. It maps the runtime
// lifecycle onto ATE's actor lifecycle:
//
//	Activate   -> CreateActor (idempotent) + ResumeActor -> worker pod IP
//	Deactivate -> SuspendActor (returns worker to the standby pool)
//	Teardown   -> best-effort suspend (actor deletion is managed out of band)
type SubstrateRuntime struct {
	ateClient *ate.Client
	port      int
}

// SubstrateConfig configures a SubstrateRuntime.
type SubstrateConfig struct {
	// Endpoint is the ATE control-plane target. Empty uses the ate client
	// default (api.ate-system.svc:443).
	Endpoint string
	// Namespace is the atespace/namespace for actors. Empty defaults to "ax".
	Namespace string
	// Template is the ActorTemplate name. Empty defaults to "ax-harness-template".
	Template string
	// Port is the HarnessService port exposed by the actor. Zero defaults to 80,
	// matching substrate actor networking (DNAT of workerPodIP:80 to the actor).
	Port int
}

// NewSubstrateRuntime creates a SubstrateRuntime from cfg.
func NewSubstrateRuntime(cfg SubstrateConfig) (*SubstrateRuntime, error) {
	port := cfg.Port
	if port == 0 {
		port = 80
	}
	namespace := cfg.Namespace
	if namespace == "" {
		namespace = "ax"
	}
	template := cfg.Template
	if template == "" {
		template = "ax-harness-template"
	}
	// NOTE: preserves the existing behavior of skipping control-plane TLS
	// verification. Tracked for hardening alongside the runtime refactor.
	controlCreds := grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true}))
	client, err := ate.NewClient(namespace, template, cfg.Endpoint, controlCreds)
	if err != nil {
		return nil, fmt.Errorf("failed to create ATE client: %w", err)
	}
	return &SubstrateRuntime{ateClient: client, port: port}, nil
}

// Name implements Runtime.
func (r *SubstrateRuntime) Name() string { return "substrate" }

// Activate creates (idempotently) and resumes the actor, returning the worker
// endpoint. The endpoint is insecure gRPC on the worker pod IP.
func (r *SubstrateRuntime) Activate(ctx context.Context, conversationID string) (*Endpoint, error) {
	if conversationID == "" {
		return nil, errors.New("substrate runtime needs a valid conversationID")
	}

	// CreateActor is idempotent here: on follow-up turns the actor already
	// exists (created and suspended on a previous turn), so AlreadyExists is
	// expected and fine.
	if _, err := r.ateClient.CreateActor(ctx, conversationID); err != nil && status.Code(err) != codes.AlreadyExists {
		return nil, fmt.Errorf("failed to create substrate actor %s: %w", conversationID, err)
	}

	resumeResp, err := r.ateClient.ResumeActor(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("failed to resume substrate actor %s: %w", conversationID, err)
	}
	actor := resumeResp.Actor
	if actor == nil {
		return nil, fmt.Errorf("received nil actor in response for %s", conversationID)
	}
	if actor.AteomPodIp == "" {
		return nil, fmt.Errorf("actor %s has no active worker IP address", conversationID)
	}

	return &Endpoint{
		Address: fmt.Sprintf("%s:%d", actor.AteomPodIp, r.port),
		UseTLS:  false,
	}, nil
}

// Deactivate suspends the actor, returning its worker to the standby pool.
func (r *SubstrateRuntime) Deactivate(ctx context.Context, conversationID string) error {
	if _, err := r.ateClient.SuspendActor(ctx, conversationID); err != nil {
		return fmt.Errorf("failed to suspend substrate actor %s: %w", conversationID, err)
	}
	return nil
}

// Teardown is best-effort. Actor deletion is currently managed out of band, so
// this suspends the actor to release worker resources.
func (r *SubstrateRuntime) Teardown(ctx context.Context, conversationID string) error {
	return r.Deactivate(ctx, conversationID)
}

// Close closes the ATE control-plane connection.
func (r *SubstrateRuntime) Close() error {
	return r.ateClient.Close()
}
