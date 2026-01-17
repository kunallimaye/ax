// Gar is a CLI tool for managing agent orchestrator sessions.
// It provides commands to trigger sessions, resume from checkpoints,
// inspect session state, register agents, and run the controller server.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "gar",
	Short: "GAR - Google Agent Runtime CLI",
	Long: `Gar is a CLI tool for managing agent orchestrator sessions.
It provides commands to trigger sessions, resume from checkpoints,
inspect session state, register agents, and run the controller server.`,
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(triggerCmd)
	rootCmd.AddCommand(inspectCmd)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(serveCmd)
}

func connect(server string) (*grpc.ClientConn, error) {
	conn, err := grpc.NewClient(server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	return conn, nil
}
