package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/google/gar/proto"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var (
	triggerSessionID  string
	triggerInput      string
	triggerCheckpoint string
	triggerServerAddr string
)

var triggerCmd = &cobra.Command{
	Use:   "trigger",
	Short: "Trigger a new session or resume an existing one",
	Long: `Trigger a new agentic session or resume an existing one.
If no session ID is provided, a new UUID will be generated.
Use --checkpoint to resume from a specific checkpoint.`,
	RunE: runTrigger,
}

func init() {
	triggerCmd.Flags().StringVar(&triggerSessionID, "session-id", "", "Session ID (optional, generates UUID if not provided)")
	triggerCmd.Flags().StringVar(&triggerInput, "input", "", "Input message to send (required)")
	triggerCmd.Flags().StringVar(&triggerCheckpoint, "checkpoint", "", "Resume from specific checkpoint UUID (empty for latest)")
	triggerCmd.Flags().StringVar(&triggerServerAddr, "server", "localhost:8494", "gRPC controller server address (default: localhost:8494)")
	triggerCmd.MarkFlagRequired("input")
}

// TODO(jbd): Add multimodal input flags, e.g. --input-image.

func runTrigger(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Generate UUID if no session ID provided
	if triggerSessionID == "" {
		triggerSessionID = uuid.New().String()
		fmt.Printf("Generated session ID: %s\n", triggerSessionID)
	}

	// Create input content
	inputs := []*proto.Content{
		{
			Role:     "user",
			Type:     "text",
			Mimetype: "text/plain",
			Data:     triggerInput,
		},
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	conn, err := connect(inspectServerAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := proto.NewGARServiceClient(conn)
	stream, err := client.TriggerSession(ctx, &proto.TriggerSessionRequest{
		SessionId:    triggerSessionID,
		Inputs:       inputs,
		CheckpointId: triggerCheckpoint,
	})
	if err != nil {
		return fmt.Errorf("error triggering session: %w", err)
	}

	// Receive and print all responses
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error receiving response: %w", err)
		}

		if resp.Output != nil {
			fmt.Printf("[%s] %s\n", resp.State, resp.Output.Data)
		}
	}
	return nil
}
