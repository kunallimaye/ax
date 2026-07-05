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

package runtime

import (
	"context"
	"testing"
)

// fakeRuntime is a minimal Runtime for registry tests.
type fakeRuntime struct{ name string }

func (f *fakeRuntime) Name() string { return f.name }
func (f *fakeRuntime) Activate(context.Context, string) (*Endpoint, error) {
	return &Endpoint{Address: "127.0.0.1:1"}, nil
}
func (f *fakeRuntime) Deactivate(context.Context, string) error { return nil }
func (f *fakeRuntime) Teardown(context.Context, string) error   { return nil }
func (f *fakeRuntime) Close() error                             { return nil }

func TestResolve_RequirementWinsOverDefault(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&fakeRuntime{name: "local"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&fakeRuntime{name: "substrate"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.SetDefault("local"); err != nil {
		t.Fatal(err)
	}

	// Requirement wins.
	rt, err := reg.Resolve("substrate")
	if err != nil {
		t.Fatalf("Resolve(substrate): %v", err)
	}
	if rt.Name() != "substrate" {
		t.Errorf("Resolve(substrate) = %q, want substrate", rt.Name())
	}

	// No requirement -> default.
	rt, err = reg.Resolve("")
	if err != nil {
		t.Fatalf("Resolve(default): %v", err)
	}
	if rt.Name() != "local" {
		t.Errorf("Resolve(\"\") = %q, want local (default)", rt.Name())
	}
}

func TestResolve_NoDefaultNoRequirement(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&fakeRuntime{name: "local"})
	if _, err := reg.Resolve(""); err == nil {
		t.Fatal("expected error when no requirement and no default configured")
	}
}

func TestSetDefault_Unregistered(t *testing.T) {
	reg := NewRegistry()
	if err := reg.SetDefault("cloudrun"); err == nil {
		t.Fatal("expected error setting default to unregistered runtime")
	}
}

func TestRegister_Duplicate(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&fakeRuntime{name: "local"})
	if err := reg.Register(&fakeRuntime{name: "local"}); err == nil {
		t.Fatal("expected error registering duplicate runtime name")
	}
}
