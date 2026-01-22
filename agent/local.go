package agent

import (
	"context"
	"fmt"

	"github.com/google/gar/proto"
)

// LocalAgent wraps a local (in-process) agent implementation.
// It implements the Agent interface for agents running in the same process as the dispatcher.
type LocalAgent struct {
	id              string
	processFunc     func(ctx context.Context, inputs []*proto.Content, handler OutputHandler) error
	lifecycleFunc   func(ctx context.Context, handler LifecycleHandler) error
	healthCheckFunc func(ctx context.Context) error
}

// LocalAgentConfig configures a local agent.
type LocalAgentConfig struct {
	ID              string
	ProcessFunc     func(ctx context.Context, inputs []*proto.Content, handler OutputHandler) error
	LifecycleFunc   func(ctx context.Context, handler LifecycleHandler) error
	HealthCheckFunc func(ctx context.Context) error
}

// NewLocalAgent creates a new local agent with the provided configuration.
func NewLocalAgent(config LocalAgentConfig) (*LocalAgent, error) {
	if config.ID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if config.ProcessFunc == nil {
		return nil, fmt.Errorf("ProcessFunc cannot be nil")
	}

	// Provide defaults for optional functions
	if config.HealthCheckFunc == nil {
		config.HealthCheckFunc = func(ctx context.Context) error { return nil }
	}
	if config.LifecycleFunc == nil {
		config.LifecycleFunc = func(ctx context.Context, handler LifecycleHandler) error {
			// Default: no lifecycle events
			return nil
		}
	}

	return &LocalAgent{
		id:              config.ID,
		processFunc:     config.ProcessFunc,
		lifecycleFunc:   config.LifecycleFunc,
		healthCheckFunc: config.HealthCheckFunc,
	}, nil
}

// Process handles processing of input content with callback handler.
func (a *LocalAgent) Process(ctx context.Context, inputs []*proto.Content, handler OutputHandler) error {
	return a.processFunc(ctx, inputs, handler)
}

// StreamLifecycle streams lifecycle events using callback handler.
func (a *LocalAgent) StreamLifecycle(ctx context.Context, handler LifecycleHandler) error {
	return a.lifecycleFunc(ctx, handler)
}

// HealthCheck checks if the agent is healthy.
func (a *LocalAgent) HealthCheck(ctx context.Context) error {
	return a.healthCheckFunc(ctx)
}

// ID returns the unique identifier for this agent.
func (a *LocalAgent) ID() string {
	return a.id
}

// Close gracefully shuts down the agent.
func (a *LocalAgent) Close() error {
	// Local agents don't typically need cleanup, but this can be extended
	return nil
}
