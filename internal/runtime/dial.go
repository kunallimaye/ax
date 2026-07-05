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
	"crypto/tls"
	"fmt"
	"os/exec"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// Dial creates a gRPC client connection to the endpoint, applying transport
// security and, when the endpoint declares an Audience, per-RPC Google identity
// token credentials (as required by an authenticated Cloud Run service).
//
// Endpoints with an Audience use ADC to mint identity tokens for that audience;
// the caller's principal (service account, or user via ADC) must have
// run.invoker on the target service.
func Dial(ctx context.Context, ep *Endpoint, extra ...grpc.DialOption) (*grpc.ClientConn, error) {
	if ep == nil {
		return nil, fmt.Errorf("nil endpoint")
	}

	opts := []grpc.DialOption{grpc.WithStatsHandler(otelgrpc.NewClientHandler())}

	if ep.UseTLS {
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})))
	} else {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	if ep.Audience != "" {
		ts, err := identityTokenSource(ctx, ep.Audience)
		if err != nil {
			return nil, fmt.Errorf("failed to create identity token source for audience %q: %w", ep.Audience, err)
		}
		opts = append(opts, grpc.WithPerRPCCredentials(&idTokenCreds{
			ts:         ts,
			requireTLS: ep.UseTLS,
		}))
	}

	opts = append(opts, extra...)

	conn, err := grpc.NewClient(ep.Address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", ep.Address, err)
	}
	return conn, nil
}

// idTokenCreds is a grpc credentials.PerRPCCredentials implementation that
// attaches a Google identity token as an Authorization: Bearer header.
type idTokenCreds struct {
	ts         oauth2.TokenSource
	requireTLS bool
}

func (c *idTokenCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	tok, err := c.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to obtain identity token: %w", err)
	}
	return map[string]string{
		"authorization": tok.Type() + " " + tok.AccessToken,
	}, nil
}

func (c *idTokenCreds) RequireTransportSecurity() bool { return c.requireTLS }

// identityTokenSource returns an oauth2.TokenSource that mints Google identity
// (OIDC) tokens for the given audience. It first tries the standard idtoken
// source (works for service accounts / metadata server). If that fails because
// ADC is user credentials ("authorized_user"), which idtoken does not support,
// it falls back to the gcloud CLI to obtain an identity token. This keeps the
// developer/interactive flow working while production (service account) uses the
// native path.
func identityTokenSource(ctx context.Context, audience string) (oauth2.TokenSource, error) {
	ts, err := idtoken.NewTokenSource(ctx, audience)
	if err == nil {
		return ts, nil
	}
	if strings.Contains(err.Error(), "authorized_user") {
		if _, lookErr := exec.LookPath("gcloud"); lookErr == nil {
			return &gcloudIDTokenSource{audience: audience}, nil
		}
	}
	return nil, err
}

// gcloudIDTokenSource obtains identity tokens via the gcloud CLI, for use when
// ADC is user credentials.
type gcloudIDTokenSource struct {
	audience string
}

func (g *gcloudIDTokenSource) Token() (*oauth2.Token, error) {
	// Prefer application-default identity token for the audience.
	out, err := exec.Command("gcloud", "auth", "print-identity-token",
		"--audiences="+g.audience).Output()
	if err != nil {
		return nil, fmt.Errorf("gcloud print-identity-token failed: %w", err)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return nil, fmt.Errorf("gcloud returned empty identity token")
	}
	return &oauth2.Token{AccessToken: tok, TokenType: "Bearer"}, nil
}
