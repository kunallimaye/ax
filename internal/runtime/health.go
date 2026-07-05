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
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// DefaultHealthCheckTimeout is the maximum time WaitForHealthy waits for a
// freshly provisioned endpoint's HarnessService to become reachable and ready.
const DefaultHealthCheckTimeout = 60 * time.Second

// WaitForHealthy blocks until the harness behind conn reports SERVING via the
// standard gRPC health protocol, or until timeout. An endpoint that is reachable
// but does not implement the health service (Unimplemented) is treated as ready;
// connection failures (Unavailable) and NOT_SERVING are retried with backoff.
func WaitForHealthy(ctx context.Context, conn *grpc.ClientConn, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultHealthCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := grpc_health_v1.NewHealthClient(conn)
	const maxBackoff = 2 * time.Second
	backoff := 100 * time.Millisecond
	for {
		resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: ""})
		if err == nil && resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING {
			return nil
		}
		if status.Code(err) == codes.Unimplemented {
			// Reachable but no health service: the port is up, proceed.
			return nil
		}

		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("harness not healthy within %s: %w", timeout, err)
			}
			return fmt.Errorf("harness not healthy within %s (last status: %s)", timeout, resp.GetStatus())
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}
