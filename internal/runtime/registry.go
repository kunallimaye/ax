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
	"fmt"
	"sync"
)

// Registry manages a collection of runtimes keyed by name, and tracks the
// configured default runtime used when a harness declares no requirement.
type Registry struct {
	mu        sync.RWMutex
	runtimes  map[string]Runtime
	defaultID string
}

// NewRegistry creates an empty runtime registry.
func NewRegistry() *Registry {
	return &Registry{
		runtimes: make(map[string]Runtime),
	}
}

// Register adds a runtime under its Name(). It returns an error if a runtime
// with the same name is already registered.
func (r *Registry) Register(rt Runtime) error {
	if rt == nil {
		return fmt.Errorf("cannot register nil runtime")
	}
	name := rt.Name()
	if name == "" {
		return fmt.Errorf("runtime name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runtimes[name]; ok {
		return fmt.Errorf("runtime %q already registered", name)
	}
	r.runtimes[name] = rt
	return nil
}

// SetDefault records the name of the default runtime. The named runtime must
// already be registered.
func (r *Registry) SetDefault(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runtimes[name]; !ok {
		return fmt.Errorf("default runtime %q is not registered", name)
	}
	r.defaultID = name
	return nil
}

// Get returns the runtime registered under name.
func (r *Registry) Get(name string) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime %q not found", name)
	}
	return rt, nil
}

// Resolve selects the runtime for a harness. A non-empty requirement (the
// per-agent runtime requirement) always wins; otherwise the configured default
// applies. It errors if neither yields a registered runtime.
func (r *Registry) Resolve(requirement string) (Runtime, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	name := requirement
	if name == "" {
		name = r.defaultID
	}
	if name == "" {
		return nil, fmt.Errorf("no runtime requirement and no default runtime configured")
	}
	rt, ok := r.runtimes[name]
	if !ok {
		return nil, fmt.Errorf("runtime %q not found (requirement=%q, default=%q)", name, requirement, r.defaultID)
	}
	return rt, nil
}

// Default returns the configured default runtime name (may be empty).
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultID
}

// Close closes all registered runtimes, returning the first error encountered.
func (r *Registry) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var firstErr error
	for name, rt := range r.runtimes {
		if err := rt.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("closing runtime %q: %w", name, err)
		}
	}
	return firstErr
}
