package main

import (
	"context"
	"fmt"
	"time"

	"github.com/google/gar/proto"
	"github.com/spf13/cobra"
)

var (
	inspectSessionID  string
	inspectServerAddr string
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect a session",
	Long:  `Inspect a session to view its current state, step count, and other details.`,
	RunE:  runInspect,
}

func init() {
	inspectCmd.Flags().StringVar(&inspectSessionID, "session-id", "", "Session ID (required)")
	inspectCmd.Flags().StringVar(&inspectServerAddr, "server", "localhost:8494", "gRPC controller server address (default: localhost:8494)")
	inspectCmd.MarkFlagRequired("session-id")
}

func runInspect(cmd *cobra.Command, args []string) error {
	fmt.Printf("Inspecting session: %s\n", inspectSessionID)

	conn, err := openConn(inspectServerAddr)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := proto.NewGARServiceClient(conn)

	// Get session details
	resp, err := client.GetSession(context.Background(), &proto.GetSessionRequest{
		SessionId: inspectSessionID,
	})
	if err != nil {
		return fmt.Errorf("error getting session: %w", err)
	}

	session := resp.Session

	// Print session details
	fmt.Println("\nSession Details:")
	fmt.Printf("  ID: %s\n", session.SessionId)
	fmt.Printf("  State: %s\n", session.State)
	fmt.Printf("  Current Step: %d\n", session.CurrentStep)
	fmt.Printf("  Created At: %s\n", time.UnixMilli(session.CreatedAt).Format(time.RFC3339))
	fmt.Printf("  Updated At: %s\n", time.UnixMilli(session.UpdatedAt).Format(time.RFC3339))
	fmt.Printf("  Message Count: %d\n", session.MessageCount)
	fmt.Printf("  Checkpoints: %d\n", session.CheckpointCount)
	fmt.Printf("  Active Agents: %v\n", session.ActiveAgents)

	return nil
}
