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

// Command adk_weather_agent is a standalone HarnessService gRPC server backed by
// a Google ADK (Agent Development Kit) for Go agent. It demonstrates plugging an
// arbitrary agent SDK into AX purely through the HarnessService contract: AX does
// not import ADK; it only speaks gRPC to this process.
//
// The agent uses a Gemini model on Vertex AI (Application Default Credentials, no
// API key) plus a get_weather function tool to answer weather questions. It is
// designed to run as a stateless Cloud Run service: each turn rehydrates any
// prior turns supplied by AX in the HarnessStart messages.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"

	"google.golang.org/genai"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"

	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"

	"github.com/google/ax/proto"
)

const appName = "adk-weather-agent"

func main() {
	ctx := context.Background()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Cloud Run default contract.
	}
	modelName := os.Getenv("ADK_MODEL")
	if modelName == "" {
		modelName = "gemini-2.5-flash"
	}
	project := firstNonEmpty(os.Getenv("GOOGLE_CLOUD_PROJECT"), os.Getenv("VERTEX_PROJECT"))
	location := firstNonEmpty(os.Getenv("GOOGLE_CLOUD_LOCATION"), os.Getenv("VERTEX_LOCATION"), "us-central1")

	rnr, err := newAgentRunner(ctx, modelName, project, location)
	if err != nil {
		log.Fatalf("failed to build agent runner: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	proto.RegisterHarnessServiceServer(grpcServer, &harnessServer{runner: rnr})

	// Register a gRPC health service so the AX runtime health gate passes.
	hs := health.NewServer()
	hs.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, hs)

	log.Printf("adk_weather_agent listening on :%s (model=%s project=%s location=%s)", port, modelName, project, location)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}

// newAgentRunner constructs an ADK LLM agent with a weather tool and a runner.
func newAgentRunner(ctx context.Context, modelName, project, location string) (*runner.Runner, error) {
	model, err := gemini.NewModel(ctx, modelName, &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  project,
		Location: location,
	})
	if err != nil {
		return nil, fmt.Errorf("create gemini model: %w", err)
	}

	weatherTool, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Get the current weather for a city. Returns a short human-readable summary.",
	}, getWeather)
	if err != nil {
		return nil, fmt.Errorf("create weather tool: %w", err)
	}

	root, err := llmagent.New(llmagent.Config{
		Name:        "weather_agent",
		Model:       model,
		Description: "Answers questions about the current weather in a city.",
		Instruction: "You are a helpful weather assistant. When the user asks about the weather in a " +
			"place, call the get_weather tool with the city name and answer concisely in one or two " +
			"sentences using the tool's result. Include the temperature and conditions.",
		Tools: []tool.Tool{weatherTool},
	})
	if err != nil {
		return nil, fmt.Errorf("create llm agent: %w", err)
	}

	rnr, err := runner.New(runner.Config{
		AppName:           appName,
		Agent:             root,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create runner: %w", err)
	}
	return rnr, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
