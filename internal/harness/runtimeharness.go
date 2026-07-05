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

package harness

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/google/ax/internal/runtime"
	"github.com/google/ax/proto"
	"github.com/google/uuid"
)

// deactivateTimeout bounds the best-effort Deactivate call made on Close.
const deactivateTimeout = 10 * time.Second

// Compile-time interface assertions.
var _ Harness = (*RuntimeHarness)(nil)
var _ Execution = (*runtimeExecution)(nil)

// RuntimeHarness is the generic harness: it describes an agent (identified by
// harnessID, speaking the HarnessService gRPC contract) and delegates the
// "where/how it runs" concern to a runtime.Runtime. This decouples the agent
// (what) from the substrate (where), so the same agent can run on Cloud Run,
// the Agent Substrate, or locally simply by pairing it with a different runtime.
type RuntimeHarness struct {
	harnessID string
	rt        runtime.Runtime
}

// NewRuntimeHarness pairs an agent id with a runtime.
func NewRuntimeHarness(harnessID string, rt runtime.Runtime) (*RuntimeHarness, error) {
	if harnessID == "" {
		return nil, fmt.Errorf("harness id is required")
	}
	if rt == nil {
		return nil, fmt.Errorf("runtime is required for harness %q", harnessID)
	}
	return &RuntimeHarness{harnessID: harnessID, rt: rt}, nil
}

// Start activates an endpoint for the conversation via the runtime, connects to
// it, and returns an Execution bound to that endpoint.
func (h *RuntimeHarness) Start(ctx context.Context, conversationID string, harnessConfig []byte) (Execution, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("RuntimeHarness needs a valid conversationID")
	}

	ep, err := h.rt.Activate(ctx, conversationID)
	if err != nil {
		return nil, fmt.Errorf("runtime %q failed to activate %s: %w", h.rt.Name(), conversationID, err)
	}

	conn, err := runtime.Dial(ctx, ep)
	if err != nil {
		return nil, fmt.Errorf("failed to dial harness endpoint %s: %w", ep.Address, err)
	}

	if err := runtime.WaitForHealthy(ctx, conn, runtime.DefaultHealthCheckTimeout); err != nil {
		conn.Close()
		return nil, fmt.Errorf("harness %q not ready at %s: %w", h.harnessID, ep.Address, err)
	}

	return &runtimeExecution{
		harness:        h,
		conversationID: conversationID,
		execID:         uuid.NewString(),
		conn:           conn,
		client:         proto.NewHarnessServiceClient(conn),
		harnessConfig:  harnessConfig,
	}, nil
}

type runtimeExecution struct {
	harness        *RuntimeHarness
	conversationID string
	execID         string
	conn           grpcClientConn
	client         proto.HarnessServiceClient
	harnessConfig  []byte

	mu      sync.Mutex
	pending []*proto.Message
}

// grpcClientConn is the minimal subset of *grpc.ClientConn used here, allowing
// the connection to be closed in Execution.Close.
type grpcClientConn interface {
	Close() error
}

func (e *runtimeExecution) ID() string { return e.execID }

func (e *runtimeExecution) Queue(ctx context.Context, msg ...*proto.Message) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, msg...)
	return nil
}

func (e *runtimeExecution) Run(ctx context.Context, handler Handler) error {
	ctx, span := otel.Tracer("runtime-harness").Start(ctx, "Run")
	defer span.End()

	e.mu.Lock()
	inputs := e.pending
	e.pending = nil
	e.mu.Unlock()

	stream, err := e.client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("failed to open harness service stream: %w", err)
	}

	start := &proto.HarnessRequest{
		ConversationId: e.conversationID,
		HarnessId:      e.harness.harnessID,
		Type: &proto.HarnessRequest_Start{
			Start: &proto.HarnessStart{
				HarnessConfig: e.harnessConfig,
				Messages:      inputs,
			},
		},
	}
	if err := stream.Send(start); err != nil {
		return fmt.Errorf("failed to send harness start: %w", err)
	}
	if err := stream.CloseSend(); err != nil {
		return fmt.Errorf("failed to close stream send direction: %w", err)
	}

	return drainStream(ctx, stream, e.execID, handler)
}

func (e *runtimeExecution) Close(ctx context.Context) error {
	if e.conn != nil {
		e.conn.Close()
	}
	slog.InfoContext(ctx, "Deactivating runtime endpoint",
		slog.String("conversation_id", e.conversationID),
		slog.String("exec_id", e.execID),
		slog.String("runtime", e.harness.rt.Name()),
	)
	deactivateCtx, cancel := context.WithTimeout(context.Background(), deactivateTimeout)
	defer cancel()
	if err := e.harness.rt.Deactivate(deactivateCtx, e.conversationID); err != nil {
		slog.ErrorContext(ctx, "Failed to deactivate runtime endpoint",
			slog.String("conversation_id", e.conversationID),
			slog.String("runtime", e.harness.rt.Name()),
			slog.Any("error", err),
		)
	}
	return nil
}
