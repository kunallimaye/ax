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

package cliutil

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/google/ax/internal/config"
	"github.com/google/ax/internal/controller"
	"github.com/google/ax/internal/controller/eventlog"
	"github.com/google/ax/internal/harness"
	"github.com/google/ax/internal/runtime"
)

const (
	antigravityHarnessID = "antigravity"
	adkHarnessID         = "adk"
)

// Controller is the active controller type for this build.
type Controller = *controller.Controller

// ExecHandler is the handler type accepted by Controller.Exec.
type ExecHandler = controller.ExecHandler

// Config is the configuration type for this build.
type Config = config.Config

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	return config.LoadFromFile(path)
}

// DefaultConfig returns a configuration with default values set.
func DefaultConfig() *Config {
	return config.DefaultConfig()
}

// NewControllerFromConfig creates a controller.Controller instance based on the
// provided configuration. It builds the runtime registry, resolves each
// harness's runtime (per-agent requirement wins over runtime.default), and
// registers the harnesses.
func NewControllerFromConfig(ctx context.Context, cfg *Config) (*controller.Controller, error) {
	runtimes, err := buildRuntimeRegistry(ctx, cfg)
	if err != nil {
		return nil, err
	}

	reg := controller.NewRegistry()
	var defaultHarnessID string

	// AX_SUBSTRATE is deprecated. If set, it forces the default runtime to
	// substrate for built-in harnesses, preserving legacy behavior with a warning.
	legacySubstrate := os.Getenv("AX_SUBSTRATE") == "1"
	if legacySubstrate {
		slog.WarnContext(ctx, "AX_SUBSTRATE=1 is deprecated; set runtime.default: substrate (and per-harness runtime:) in config instead. Applying legacy substrate default for built-in harnesses.")
	}

	// Built-in: Antigravity.
	if cfg.Harnesses.Antigravity.Enabled || cfg.Harnesses.Antigravity.Default || legacySubstrate {
		req := cfg.Harnesses.Antigravity.Runtime
		if legacySubstrate && req == "" {
			req = config.RuntimeSubstrate
		}
		rt, err := runtimes.Resolve(req)
		if err != nil {
			return nil, fmt.Errorf("antigravity harness: %w", err)
		}
		h, err := harness.NewRuntimeHarness(antigravityHarnessID, rt)
		if err != nil {
			return nil, fmt.Errorf("antigravity harness: %w", err)
		}
		if err := reg.RegisterHarness(antigravityHarnessID, h); err != nil {
			return nil, fmt.Errorf("register antigravity harness: %w", err)
		}
		if cfg.Harnesses.Antigravity.Default {
			defaultHarnessID = antigravityHarnessID
		}
	}

	// Built-in: ADK.
	if cfg.Harnesses.ADK.Enabled || cfg.Harnesses.ADK.Default {
		rt, err := runtimes.Resolve(cfg.Harnesses.ADK.Runtime)
		if err != nil {
			return nil, fmt.Errorf("adk harness: %w", err)
		}
		h, err := harness.NewRuntimeHarness(adkHarnessID, rt)
		if err != nil {
			return nil, fmt.Errorf("adk harness: %w", err)
		}
		if err := reg.RegisterHarness(adkHarnessID, h); err != nil {
			return nil, fmt.Errorf("register adk harness: %w", err)
		}
		if cfg.Harnesses.ADK.Default {
			defaultHarnessID = adkHarnessID
		}
	}

	// Custom substrate harnesses.
	for _, sc := range cfg.Harnesses.Substrate {
		sr, err := runtime.NewSubstrateRuntime(runtime.SubstrateConfig{
			Endpoint:  cfg.Runtime.Substrate.ControlEndpoint,
			Namespace: sc.Namespace,
			Template:  sc.Template,
			Port:      sc.Port,
		})
		if err != nil {
			return nil, fmt.Errorf("substrate harness %q: %w", sc.ID, err)
		}
		h, err := harness.NewRuntimeHarness(sc.ID, sr)
		if err != nil {
			return nil, fmt.Errorf("substrate harness %q: %w", sc.ID, err)
		}
		if err := reg.RegisterHarness(sc.ID, h); err != nil {
			return nil, fmt.Errorf("register substrate harness %q: %w", sc.ID, err)
		}
		if sc.Default {
			defaultHarnessID = sc.ID
		}
	}

	// Register the configured default harness under the empty id.
	if defaultHarnessID != "" {
		h, err := reg.Harness(defaultHarnessID)
		if err != nil {
			return nil, fmt.Errorf("default harness %q not found", defaultHarnessID)
		}
		if err := reg.RegisterHarness("", h); err != nil {
			return nil, fmt.Errorf("register default harness %q: %w", defaultHarnessID, err)
		}
	}

	return controller.New(ctx, controller.Config{
		Registry: reg,
		EventLogBuilder: func() (eventlog.EventLog, error) {
			if cfg.EventLog.PostgresConfig.DSN != "" {
				dsn := os.ExpandEnv(cfg.EventLog.PostgresConfig.DSN)
				if dsn == "" {
					return nil, fmt.Errorf("eventlog: postgres dsn %q expanded to empty", cfg.EventLog.PostgresConfig.DSN)
				}
				return eventlog.OpenPostgresEventLog(dsn)
			}
			return eventlog.OpenSQLiteEventLog(cfg.EventLog.SQLiteConfig.Filename)
		},
	})
}

// buildRuntimeRegistry constructs and registers all runtimes referenced by the
// configuration, and sets the default. Runtimes are constructed lazily: only the
// default runtime and any runtime referenced by an enabled harness requirement
// are built, so a purely-local run needs no GCP credentials.
func buildRuntimeRegistry(ctx context.Context, cfg *Config) (*runtime.Registry, error) {
	reg := runtime.NewRegistry()
	needed := map[string]bool{}

	def := cfg.Runtime.Default
	if def == "" {
		def = config.RuntimeLocal
	}
	needed[def] = true

	// Legacy env override.
	if os.Getenv("AX_SUBSTRATE") == "1" {
		needed[config.RuntimeSubstrate] = true
	}

	// Per-harness requirements.
	for _, req := range []string{cfg.Harnesses.Antigravity.Runtime, cfg.Harnesses.ADK.Runtime} {
		if req != "" {
			needed[req] = true
		}
	}

	if needed[config.RuntimeLocal] {
		addr := cfg.Runtime.Local.Address
		if addr == "" {
			addr = cfg.Harnesses.Antigravity.Endpoint // back-compat: antigravity endpoint
		}
		if err := reg.Register(runtime.NewLocalRuntime(addr)); err != nil {
			return nil, err
		}
	}

	if needed[config.RuntimeSubstrate] {
		sr, err := runtime.NewSubstrateRuntime(runtime.SubstrateConfig{
			Endpoint:  cfg.Runtime.Substrate.ControlEndpoint,
			Namespace: cfg.Runtime.Substrate.Namespace,
			Template:  cfg.Runtime.Substrate.Template,
			Port:      cfg.Runtime.Substrate.Port,
		})
		if err != nil {
			return nil, fmt.Errorf("build substrate runtime: %w", err)
		}
		if err := reg.Register(sr); err != nil {
			return nil, err
		}
	}

	if needed[config.RuntimeCloudRun] {
		cr, err := runtime.NewCloudRunRuntime(ctx, runtime.CloudRunConfig{
			Project:              cfg.Runtime.CloudRun.Project,
			Region:               cfg.Runtime.CloudRun.Region,
			Service:              cfg.Runtime.CloudRun.Service,
			AllowUnauthenticated: cfg.Runtime.CloudRun.AllowUnauthenticated,
		})
		if err != nil {
			return nil, fmt.Errorf("build cloudrun runtime: %w", err)
		}
		if err := reg.Register(cr); err != nil {
			return nil, err
		}
	}

	if err := reg.SetDefault(def); err != nil {
		return nil, err
	}
	return reg, nil
}
