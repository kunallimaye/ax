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
	"fmt"
	"net/url"
	"strings"
	"sync"

	run "google.golang.org/api/run/v2"
)

// Compile-time interface assertion.
var _ Runtime = (*CloudRunRuntime)(nil)

// CloudRunRuntime provisions agent endpoints as Cloud Run services following the
// stateless service-per-template model:
//
//   - Template  -> one Cloud Run service (the agent "catalog entry").
//   - Activate  -> ensure the service exists, resolve its HTTPS URL, and return
//     a TLS gRPC endpoint whose Audience is the service URL (for ID-token auth).
//     Cloud Run cold-starts an instance on the first request ("resume").
//   - Deactivate-> no-op: Cloud Run scales to zero when idle ("suspend").
//   - Teardown  -> optional deletion of the service.
//
// State is externalized (event log + object store); instances are stateless and
// interchangeable, so Activate does not pin a conversation to a specific instance.
type CloudRunRuntime struct {
	project string
	region  string
	// service is the Cloud Run service name that backs this runtime's agent
	// template. In the service-per-template model a CloudRunRuntime instance is
	// bound to exactly one service.
	service string

	// allowUnauthenticated, when true, means the service permits unauthenticated
	// invocations and the endpoint will carry no ID-token audience.
	allowUnauthenticated bool

	mu         sync.Mutex
	cachedURL  string
	runService *run.Service
}

// CloudRunConfig configures a CloudRunRuntime.
type CloudRunConfig struct {
	Project              string // GCP project ID (required).
	Region               string // Cloud Run region, e.g. "us-central1" (required).
	Service              string // Cloud Run service name backing the agent template (required).
	AllowUnauthenticated bool   // If true, do not attach an ID token.
}

// NewCloudRunRuntime creates a CloudRunRuntime. It constructs a Cloud Run Admin
// API client using Application Default Credentials.
func NewCloudRunRuntime(ctx context.Context, cfg CloudRunConfig) (*CloudRunRuntime, error) {
	if cfg.Project == "" {
		return nil, fmt.Errorf("cloudrun runtime: project is required")
	}
	if cfg.Region == "" {
		return nil, fmt.Errorf("cloudrun runtime: region is required")
	}
	if cfg.Service == "" {
		return nil, fmt.Errorf("cloudrun runtime: service is required")
	}
	svc, err := run.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("cloudrun runtime: failed to create Cloud Run client: %w", err)
	}
	return &CloudRunRuntime{
		project:              cfg.Project,
		region:               cfg.Region,
		service:              cfg.Service,
		allowUnauthenticated: cfg.AllowUnauthenticated,
		runService:           svc,
	}, nil
}

// Name implements Runtime.
func (r *CloudRunRuntime) Name() string { return "cloudrun" }

// Activate resolves the Cloud Run service URL and returns a TLS gRPC endpoint.
// The conversationID is not used to pin an instance: the service is stateless and
// rehydrates conversation state from the event log on each turn.
func (r *CloudRunRuntime) Activate(ctx context.Context, conversationID string) (*Endpoint, error) {
	svcURL, err := r.serviceURL(ctx)
	if err != nil {
		return nil, err
	}

	// Convert the HTTPS URL to a gRPC dial target. Cloud Run terminates TLS on
	// :443 and supports HTTP/2 (h2c is proxied), so gRPC-over-TLS to host:443
	// is the correct dial target.
	u, err := url.Parse(svcURL)
	if err != nil {
		return nil, fmt.Errorf("cloudrun runtime: invalid service URL %q: %w", svcURL, err)
	}
	host := u.Hostname()
	if host == "" {
		return nil, fmt.Errorf("cloudrun runtime: could not derive host from %q", svcURL)
	}

	ep := &Endpoint{
		Address: host + ":443",
		UseTLS:  true,
	}
	if !r.allowUnauthenticated {
		// Cloud Run ID-token audience is the service base URL (scheme+host).
		ep.Audience = strings.TrimSuffix(svcURL, "/")
	}
	return ep, nil
}

// serviceURL fetches (and caches) the Cloud Run service's URI.
func (r *CloudRunRuntime) serviceURL(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cachedURL != "" {
		return r.cachedURL, nil
	}
	name := fmt.Sprintf("projects/%s/locations/%s/services/%s", r.project, r.region, r.service)
	svc, err := r.runService.Projects.Locations.Services.Get(name).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("cloudrun runtime: failed to get service %q: %w", name, err)
	}
	if svc.Uri == "" {
		return "", fmt.Errorf("cloudrun runtime: service %q has no URI (not deployed?)", name)
	}
	r.cachedURL = svc.Uri
	return svc.Uri, nil
}

// Deactivate is a no-op: Cloud Run autoscaling returns idle services to zero.
func (r *CloudRunRuntime) Deactivate(ctx context.Context, conversationID string) error { return nil }

// Teardown is a no-op at the conversation level: in the service-per-template
// model, many conversations share one stateless service, so a single
// conversation's teardown must not delete the shared service. Service lifecycle
// is managed by infra automation (scripts/), not per-conversation.
func (r *CloudRunRuntime) Teardown(ctx context.Context, conversationID string) error { return nil }

// Close releases the Cloud Run client (no persistent resources to free).
func (r *CloudRunRuntime) Close() error { return nil }
