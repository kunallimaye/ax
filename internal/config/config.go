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

// Package config provides configuration for the controller server path.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	// defaultNamespace is the substrate namespace reserved for AX's built-in
	// harnesses.
	defaultNamespace = "ax"
	// substrateDefaultPort is the port for harnesses running as substrate
	// actors. Substrate's actor networking DNATs inbound workerPodIP:80 to the
	// actor.
	substrateDefaultPort = 80

	// Runtime names.
	RuntimeLocal     = "local"
	RuntimeSubstrate = "substrate"
	RuntimeCloudRun  = "cloudrun"
)

// Config represents the main configuration for the AX harness server.
type Config struct {
	Version   string          `yaml:"version"`
	Server    ServerConfig    `yaml:"server"`
	EventLog  EventLogConfig  `yaml:"eventlog"`
	Runtime   RuntimeConfig   `yaml:"runtime,omitempty"`
	Harnesses HarnessesConfig `yaml:"harnesses,omitempty"`
	Telemetry TelemetryConfig `yaml:"telemetry,omitempty"`
}

// ServerConfig configures the gRPC server.
type ServerConfig struct {
	Address string `yaml:"address"` // Server address to listen on (e.g., ":8494")
}

// TelemetryConfig configures telemetry options.
type TelemetryConfig struct {
	OTLP OTLPConfig `yaml:"otlp,omitempty"`
}

// OTLPConfig configures the OTLP exporter.
type OTLPConfig struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"` // OTLP collector endpoint (e.g., "localhost:4317")
}

// SQLiteConfig configures the SQLite event log file.
type SQLiteConfig struct {
	Filename string `yaml:"filename"` // SQLite file for event log storage
}

// PostgresConfig configures the Postgres event log.
type PostgresConfig struct {
	DSN string `yaml:"dsn"` // Postgres connection DSN
}

// EventLogConfig configures the event log storage.
type EventLogConfig struct {
	SQLiteConfig   SQLiteConfig   `yaml:"sqlite,omitempty"`
	PostgresConfig PostgresConfig `yaml:"postgres,omitempty"`
}

// RuntimeConfig configures the available runtimes (substrates) and selects the
// default. A harness may override the default by declaring its own runtime
// requirement; the per-harness requirement always wins.
type RuntimeConfig struct {
	// Default is the runtime used when a harness declares no requirement.
	// One of: "local", "substrate", "cloudrun". Empty implies "local".
	Default string `yaml:"default,omitempty"`

	CloudRun  CloudRunRuntimeConfig  `yaml:"cloudrun,omitempty"`
	Substrate SubstrateRuntimeConfig `yaml:"substrate,omitempty"`
	Local     LocalRuntimeConfig     `yaml:"local,omitempty"`
}

// CloudRunRuntimeConfig configures the Cloud Run runtime.
type CloudRunRuntimeConfig struct {
	Project string `yaml:"project,omitempty"` // GCP project ID
	Region  string `yaml:"region,omitempty"`  // Cloud Run region, e.g. "us-central1"
	Service string `yaml:"service,omitempty"` // Cloud Run service name backing the agent template
	// AllowUnauthenticated, when true, indicates the target Cloud Run service
	// permits unauthenticated invocations, so AX will not attach an identity
	// token. Default (false) uses IAM ID-token auth (production-recommended).
	AllowUnauthenticated bool `yaml:"allowUnauthenticated,omitempty"`
}

// SubstrateRuntimeConfig configures the Agent Substrate (ATE) runtime.
type SubstrateRuntimeConfig struct {
	ControlEndpoint string `yaml:"controlEndpoint,omitempty"` // ATE control-plane target
	Namespace       string `yaml:"namespace,omitempty"`       // Atespace/namespace for actors
	Template        string `yaml:"template,omitempty"`        // ActorTemplate name
	Port            int    `yaml:"port,omitempty"`            // HarnessService port on the actor
}

// LocalRuntimeConfig configures the local (fixed-address) runtime.
type LocalRuntimeConfig struct {
	Address string `yaml:"address,omitempty"` // HarnessService address, e.g. "127.0.0.1:50053"
}

// HarnessesConfig groups harnesses to serve. There are two categories:
//   - Built-in harnesses (e.g. Antigravity, ADK) whose implementation and
//     container image are provided by AX.
//   - Custom harnesses on substrate whose implementation and container image are
//     provided by the user via their own ActorTemplate.
type HarnessesConfig struct {
	Antigravity AntigravityHarnessConfig `yaml:"antigravity,omitempty"`
	ADK         ADKHarnessConfig         `yaml:"adk,omitempty"`
	Substrate   []SubstrateHarnessConfig `yaml:"substrate,omitempty"`
}

// AntigravityHarnessConfig registers the built-in Antigravity harness.
type AntigravityHarnessConfig struct {
	Enabled  bool   `yaml:"enabled,omitempty"`
	Default  bool   `yaml:"default,omitempty"`
	Endpoint string `yaml:"endpoint,omitempty"` // HarnessService address (local runtime)
	Runtime  string `yaml:"runtime,omitempty"`  // Optional per-agent runtime requirement
}

// ADKHarnessConfig registers the built-in ADK (Agent Development Kit) harness.
// The ADK agent runs as a container implementing the HarnessService gRPC
// contract. It defaults to the configured default runtime unless it declares a
// runtime requirement.
type ADKHarnessConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"`
	Default bool   `yaml:"default,omitempty"`
	Image   string `yaml:"image,omitempty"`   // Container image for the ADK agent
	Runtime string `yaml:"runtime,omitempty"` // Optional per-agent runtime requirement
}

// SubstrateHarnessConfig registers a custom harness deployed on substrate
// from a user-provided container image.
type SubstrateHarnessConfig struct {
	ID        string `yaml:"id"`                // Unique harness identifier
	Namespace string `yaml:"namespace"`         // ActorTemplate namespace (user-owned, not "ax")
	Template  string `yaml:"template"`          // ActorTemplate name
	Port      int    `yaml:"port,omitempty"`    // HarnessService port
	Default   bool   `yaml:"default,omitempty"` // Default harness or not
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.setDefaults()

	return &cfg, nil
}

// DefaultConfig returns a configuration with default values set.
func DefaultConfig() *Config {
	var cfg Config
	cfg.setDefaults()
	return &cfg
}

// setDefaults sets default values for optional fields.
func (c *Config) setDefaults() {
	if c.Server.Address == "" {
		c.Server.Address = ":8494"
	}
	if c.EventLog.SQLiteConfig.Filename == "" {
		c.EventLog.SQLiteConfig.Filename = "eventlog/log.sqlite"
	}
	if c.Runtime.Default == "" {
		c.Runtime.Default = RuntimeLocal
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.Server.Address == "" {
		return fmt.Errorf("server.address is required")
	}
	if c.EventLog.PostgresConfig.DSN == "" && c.EventLog.SQLiteConfig.Filename == "" {
		return fmt.Errorf("eventlog requires either postgres.dsn or sqlite.filename")
	}

	// Validate the default runtime name.
	switch c.Runtime.Default {
	case RuntimeLocal, RuntimeSubstrate, RuntimeCloudRun:
	default:
		return fmt.Errorf("runtime.default %q is invalid (want one of local, substrate, cloudrun)", c.Runtime.Default)
	}

	// If cloudrun is the default (or referenced), it needs project+region+service.
	if c.Runtime.Default == RuntimeCloudRun {
		if err := c.validateCloudRun(); err != nil {
			return err
		}
	}

	if err := validateRuntimeRequirement("antigravity", c.Harnesses.Antigravity.Runtime); err != nil {
		return err
	}
	if err := validateRuntimeRequirement("adk", c.Harnesses.ADK.Runtime); err != nil {
		return err
	}

	var defaultCount int
	if c.Harnesses.Antigravity.Default {
		defaultCount++
	}
	if c.Harnesses.ADK.Default {
		defaultCount++
	}

	for _, sc := range c.Harnesses.Substrate {
		if sc.ID == "" {
			return fmt.Errorf("substrate harness id is required")
		}
		if sc.ID == "antigravity" {
			return fmt.Errorf("substrate harness id %q is reserved for the built-in antigravity harness", sc.ID)
		}
		if sc.ID == "adk" {
			return fmt.Errorf("substrate harness id %q is reserved for the built-in adk harness", sc.ID)
		}
		if sc.Namespace == "" {
			return fmt.Errorf("substrate harness %q: namespace is required", sc.ID)
		}
		if sc.Namespace == defaultNamespace {
			return fmt.Errorf("substrate harness %q: namespace %q is reserved for built-in harnesses", sc.ID, defaultNamespace)
		}
		if sc.Template == "" {
			return fmt.Errorf("substrate harness %q: template is required", sc.ID)
		}
		if sc.Default {
			defaultCount++
		}
	}

	if defaultCount > 1 {
		return fmt.Errorf("multiple harnesses marked as default")
	}

	return nil
}

func (c *Config) validateCloudRun() error {
	cr := c.Runtime.CloudRun
	if cr.Project == "" {
		return fmt.Errorf("runtime.cloudrun.project is required when cloudrun runtime is used")
	}
	if cr.Region == "" {
		return fmt.Errorf("runtime.cloudrun.region is required when cloudrun runtime is used")
	}
	if cr.Service == "" {
		return fmt.Errorf("runtime.cloudrun.service is required when cloudrun runtime is used")
	}
	return nil
}

func validateRuntimeRequirement(harnessID, req string) error {
	switch req {
	case "", RuntimeLocal, RuntimeSubstrate, RuntimeCloudRun:
		return nil
	default:
		return fmt.Errorf("harness %q: runtime requirement %q is invalid (want one of local, substrate, cloudrun)", harnessID, req)
	}
}

// SubstrateDefaultPort returns the default substrate port for use by wiring code.
func SubstrateDefaultPort() int { return substrateDefaultPort }
