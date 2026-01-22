// Gar is a CLI tool for managing agent orchestrator sessions.
// It provides commands to trigger sessions, resume from checkpoints,
// inspect session state, register agents, and run the controller server.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/google/uuid"
	"github.com/google/gar/internal/controller"
	"github.com/google/gar/internal/eventlog"
	"github.com/google/gar/internal/server"
	"github.com/google/gar/proto"
)

var (
	command          string
	sessionID        string
	eventLogDir      string
	input            string
	checkpointID     string
	agentID          string
	agentName        string
	agentDescription string
	agentAddr        string
	serveAddr        string
	serverAddr       string // gRPC controller server address
)

func main() {
	// Define subcommands
	triggerCmd := flag.NewFlagSet("trigger", flag.ExitOnError)
	inspectCmd := flag.NewFlagSet("inspect", flag.ExitOnError)
	registerCmd := flag.NewFlagSet("register", flag.ExitOnError)
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)

	// Trigger command flags
	triggerCmd.StringVar(&sessionID, "session-id", "", "Session ID (optional, generates UUID if not provided, or resumes existing)")
	triggerCmd.StringVar(&input, "input", "", "Input message to send")
	triggerCmd.StringVar(&checkpointID, "checkpoint", "", "Resume from specific checkpoint UUID (empty for latest)")
	triggerCmd.StringVar(&serverAddr, "server", "", "gRPC controller server address (e.g., localhost:8494)")

	// Inspect command flags
	inspectCmd.StringVar(&sessionID, "session-id", "", "Session ID (required)")
	inspectCmd.StringVar(&serverAddr, "server", "", "gRPC controller server address (e.g., localhost:8494)")

	// Register command flags
	registerCmd.StringVar(&agentID, "agent-id", "", "Agent ID (required)")
	registerCmd.StringVar(&agentName, "name", "", "Agent name")
	registerCmd.StringVar(&agentDescription, "description", "", "Agent description")
	registerCmd.StringVar(&agentAddr, "agent-addr", "", "Agent address (e.g., localhost:50051)")
	registerCmd.StringVar(&serverAddr, "server", "", "gRPC controller server address (e.g., localhost:8494)")

	// Serve command flags
	serveCmd.StringVar(&serveAddr, "addr", ":8494", "Server address to listen on")
	serveCmd.StringVar(&eventLogDir, "eventlog-dir", "eventlog", "Event log directory")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command = os.Args[1]

	switch command {
	case "trigger":
		triggerCmd.Parse(os.Args[2:])
		runTrigger()

	case "inspect":
		inspectCmd.Parse(os.Args[2:])
		if sessionID == "" {
			fmt.Println("Error: --session-id is required")
			inspectCmd.Usage()
			os.Exit(1)
		}
		runInspect()

	case "register":
		registerCmd.Parse(os.Args[2:])
		if agentID == "" || agentAddr == "" {
			fmt.Println("Error: --agent-id and --agent-addr are required")
			registerCmd.Usage()
			os.Exit(1)
		}
		runRegister()

	case "serve":
		serveCmd.Parse(os.Args[2:])
		runServe()

	default:
		printUsage()
		os.Exit(1)
	}
}

func eventLogFactory(sessionID, eventLogDir string) controller.EventLogFactory {
	return func(sessionID string) (eventlog.EventLog, error) {
		return eventlog.NewFileEventLog(eventlog.FileConfig{
			SessionID: sessionID,
			Dir:       eventLogDir,
		})
	}
}

// connectToServer creates a gRPC client connection to the controller server.
func connectToServer() (proto.GARServiceClient, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to server: %w", err)
	}
	client := proto.NewGARServiceClient(conn)
	return client, conn, nil
}

func printUsage() {
	fmt.Println("Usage: gar <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  trigger    Trigger a new session or resume an existing one")
	fmt.Println("  list       List all sessions")
	fmt.Println("  inspect    Inspect a session")
	fmt.Println("  register   Register a remote agent")
	fmt.Println("  serve      Run controller as a gRPC server")
	fmt.Println()
	fmt.Println("Use 'gar <command> -h' for more information about a command.")
}

func runTrigger() {
	// Require server address
	if serverAddr == "" {
		fmt.Println("Error: --server flag is required")
		os.Exit(1)
	}

	// Generate UUID if no session ID provided
	if sessionID == "" {
		sessionID = uuid.New().String()
		fmt.Printf("Generated session ID: %s\n", sessionID)
	}

	fmt.Printf("Triggering session: %s\n", sessionID)

	// Create input content
	var inputs []*proto.Content
	if input != "" {
		inputs = []*proto.Content{
			{
				Role:     "user",
				Type:     "text",
				Mimetype: "text/plain",
				Data:     input,
			},
		}
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	// Connect to gRPC server
	client, conn, err := connectToServer()
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	stream, err := client.TriggerSession(ctx, &proto.TriggerSessionRequest{
		SessionId:    sessionID,
		Inputs:       inputs,
		CheckpointId: checkpointID,
	})
	if err != nil {
		fmt.Printf("Error triggering session: %v\n", err)
		os.Exit(1)
	}

	// Receive and print all responses
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("Error receiving response: %v\n", err)
			os.Exit(1)
		}

		if resp.Output != nil {
			fmt.Printf("[%s] %s\n", resp.State, resp.Output.Data)
		}
	}

	fmt.Println("Session completed successfully")
}

func runInspect() {
	// Require server address
	if serverAddr == "" {
		fmt.Println("Error: --server flag is required")
		os.Exit(1)
	}

	fmt.Printf("Inspecting session: %s\n", sessionID)

	// Connect to gRPC server
	client, conn, err := connectToServer()
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Get session details
	resp, err := client.GetSession(context.Background(), &proto.GetSessionRequest{
		SessionId: sessionID,
	})
	if err != nil {
		fmt.Printf("Error getting session: %v\n", err)
		os.Exit(1)
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
}

func runRegister() {
	// Require server address
	if serverAddr == "" {
		fmt.Println("Error: --server flag is required")
		os.Exit(1)
	}

	fmt.Printf("Registering agent: %s at %s\n", agentID, agentAddr)
	if agentName != "" {
		fmt.Printf("  Name: %s\n", agentName)
	}
	if agentDescription != "" {
		fmt.Printf("  Description: %s\n", agentDescription)
	}

	// Connect to gRPC server
	client, conn, err := connectToServer()
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	// Register remote agent
	_, err = client.RegisterAgent(context.Background(), &proto.RegisterAgentRequest{
		AgentId:     agentID,
		AgentType:   "remote",
		Name:        agentName,
		Description: agentDescription,
		Address:     agentAddr,
	})
	if err != nil {
		fmt.Printf("Error registering agent: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Agent registered successfully")
}

func runServe() {
	fmt.Printf("Starting controller server on %s\n", serveAddr)

	c, err := newController()
	if err != nil {
		fmt.Printf("Error creating controller: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	// Create server
	srv := server.New(c)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nReceived interrupt, shutting down...")
		os.Exit(0)
	}()

	// Start serving
	if err := srv.Serve(serveAddr); err != nil {
		fmt.Printf("Error serving: %v\n", err)
		os.Exit(1)
	}
}

func newController() (*controller.Controller, error) {
	c, err := controller.New(controller.Config{
		EventLogFactory:     eventLogFactory(sessionID, eventLogDir),
		MaxSteps:            100,
		HealthCheckInterval: 30 * time.Second,
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
