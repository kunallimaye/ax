package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/google/gar/agent"
	"github.com/google/gar/internal/controller"
	"github.com/google/gar/internal/eventlog"
	"github.com/google/gar/proto"
)

func main() {
	fmt.Println("Local Agent Example")
	fmt.Println("===================")

	// Create a local echo agent
	echoAgent, err := createEchoAgent("local-echo-agent")
	if err != nil {
		fmt.Printf("Error creating agent: %v\n", err)
		return
	}

	// Create controller
	c, err := controller.New(controller.Config{
		EventLogFactory: func(sessionID string) (eventlog.EventLog, error) {
			return eventlog.NewFileEventLog(eventlog.FileConfig{
				SessionID: sessionID,
				Dir:       "eventlog",
			})
		},
		MaxSteps:            10,
		HealthCheckInterval: 30 * time.Second,
	})
	if err != nil {
		fmt.Printf("Error creating controller: %v\n", err)
		return
	}
	defer c.Close()

	// Register the local agent
	if err := c.Registry().RegisterLocal(echoAgent, "Echo Agent", "Simple echo agent that uppercases input", map[string]string{
		"type":    "echo",
		"version": "1.0",
	}); err != nil {
		fmt.Printf("Error registering agent: %v\n", err)
		return
	}

	fmt.Println("Agent registered successfully")

	// Create input content
	inputs := []*proto.Content{
		{
			Role:     "user",
			Type:     "text",
			Mimetype: "text/plain",
			Data:     "Hello, echo agent!",
		},
	}

	// Trigger a session
	sessionID := uuid.New().String()
	fmt.Printf("\nTriggering session: %s\n", sessionID)

	ctx := context.Background()
	if err := c.TriggerSession(ctx, sessionID, inputs, ""); err != nil {
		fmt.Printf("Error triggering session: %v\n", err)
		return
	}

	fmt.Println("\nSession completed!")

	// Inspect the session
	session, err := c.GetSession(sessionID)
	if err != nil {
		fmt.Printf("Error getting session: %v\n", err)
		return
	}

	fmt.Printf("\nSession state: %s\n", session.State)
	fmt.Printf("Total messages: %d\n", len(session.MessageHistory))

	fmt.Println("\nMessage history:")
	for i, msg := range session.MessageHistory {
		fmt.Printf("  %d. [%s] %s: %s\n", i+1, msg.Type, msg.Role, msg.Data)
	}
}

// createEchoAgent creates a simple echo agent that repeats input with a prefix.
func createEchoAgent(id string) (*agent.LocalAgent, error) {
	processFunc := func(ctx context.Context, inputs []*proto.Content, handler agent.OutputHandler) error {
		// Process each input and call handler with response
		for _, content := range inputs {
			// Echo the content back with a prefix
			response := &proto.Content{
				Role:     "assistant",
				Type:     content.Type,
				Mimetype: content.Mimetype,
				Data:     fmt.Sprintf("Echo: %s", strings.ToUpper(content.Data)),
			}

			// Call handler with the response
			if err := handler(response); err != nil {
				return err
			}

			// Check for context cancellation
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}

		return nil
	}

	lifecycleFunc := func(ctx context.Context, handler agent.LifecycleHandler) error {
		// Send initial PROGRESS event
		if err := handler(&proto.LifecycleEvent{
			EventType: "PROGRESS",
			AgentId:   id,
			Timestamp: time.Now().UnixMilli(),
			Metadata: map[string]string{
				"status": "started",
			},
		}); err != nil {
			return err
		}

		// Send periodic heartbeats
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := handler(&proto.LifecycleEvent{
					EventType: "PROGRESS",
					AgentId:   id,
					Timestamp: time.Now().UnixMilli(),
					Metadata: map[string]string{
						"status": "processing",
					},
				}); err != nil {
					return err
				}
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	healthCheckFunc := func(ctx context.Context) error {
		// Always healthy
		return nil
	}

	return agent.NewLocalAgent(agent.LocalAgentConfig{
		ID:              id,
		ProcessFunc:     processFunc,
		LifecycleFunc:   lifecycleFunc,
		HealthCheckFunc: healthCheckFunc,
	})
}
