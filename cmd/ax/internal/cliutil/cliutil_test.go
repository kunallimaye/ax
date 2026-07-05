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
	"path/filepath"
	"testing"

	"github.com/google/ax/internal/config"
)

func TestNewControllerFromConfig_DefaultHarness(t *testing.T) {
	cfg := &config.Config{
		EventLog: config.EventLogConfig{
			SQLiteConfig: config.SQLiteConfig{
				Filename: filepath.Join(t.TempDir(), "log.sqlite"),
			},
		},
		Runtime: config.RuntimeConfig{
			Default: config.RuntimeLocal,
			Local:   config.LocalRuntimeConfig{Address: "localhost:50053"},
		},
		Harnesses: config.HarnessesConfig{
			Antigravity: config.AntigravityHarnessConfig{
				Default:  true,
				Endpoint: "localhost:50053",
			},
		},
	}

	c, err := NewControllerFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewControllerFromConfig: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	c.Close()
}

func TestNewControllerFromConfig_ADKHarnessOnLocal(t *testing.T) {
	cfg := &config.Config{
		EventLog: config.EventLogConfig{
			SQLiteConfig: config.SQLiteConfig{
				Filename: filepath.Join(t.TempDir(), "log.sqlite"),
			},
		},
		Runtime: config.RuntimeConfig{
			Default: config.RuntimeLocal,
			Local:   config.LocalRuntimeConfig{Address: "localhost:50053"},
		},
		Harnesses: config.HarnessesConfig{
			ADK: config.ADKHarnessConfig{Enabled: true, Default: true},
		},
	}

	c, err := NewControllerFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewControllerFromConfig: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	c.Close()
}

func TestNewControllerFromConfig_BuiltinSubstrateViaLegacyEnv(t *testing.T) {
	// AX_SUBSTRATE=1 is deprecated but still forces built-in harnesses onto the
	// substrate runtime for backward compatibility.
	t.Setenv("AX_SUBSTRATE", "1")

	cfg := &config.Config{
		EventLog: config.EventLogConfig{
			SQLiteConfig: config.SQLiteConfig{
				Filename: filepath.Join(t.TempDir(), "log.sqlite"),
			},
		},
		Runtime: config.RuntimeConfig{Default: config.RuntimeLocal},
		Harnesses: config.HarnessesConfig{
			Antigravity: config.AntigravityHarnessConfig{
				Default: true,
			},
		},
	}

	c, err := NewControllerFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewControllerFromConfig: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	c.Close()
}

func TestNewControllerFromConfig_CustomHarnessOnSubstrateRuntime(t *testing.T) {
	// In the new design custom substrate harnesses always run on the substrate
	// runtime by construction; no AX_SUBSTRATE=1 is required.
	t.Setenv("AX_SUBSTRATE", "")

	cfg := &config.Config{
		EventLog: config.EventLogConfig{
			SQLiteConfig: config.SQLiteConfig{
				Filename: filepath.Join(t.TempDir(), "log.sqlite"),
			},
		},
		Runtime: config.RuntimeConfig{Default: config.RuntimeLocal},
		Harnesses: config.HarnessesConfig{
			Substrate: []config.SubstrateHarnessConfig{
				{ID: "custom", Namespace: "team-ns", Template: "custom-template"},
			},
		},
	}

	c, err := NewControllerFromConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("NewControllerFromConfig: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil controller")
	}
	c.Close()
}
