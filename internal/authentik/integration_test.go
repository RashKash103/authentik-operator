/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package authentik_test

import (
	"context"
	"os"
	"testing"
	"time"

	"rka.sh/authentik-operator/internal/authentik"
)

// These tests run against a real authentik. Start one with
// `just authentik-up`, then:
//
//	export AUTHENTIK_URL=http://localhost:9000
//	export AUTHENTIK_TOKEN=authentik-operator-e2e-bootstrap-token
//	go test ./internal/authentik/ -run TestIntegration -v
//
// Without those variables, or under -short, they skip. Everything here is
// read-only: nothing is created, mutated or deleted in the instance.
func liveClient(t *testing.T) authentik.Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	url, token := os.Getenv("AUTHENTIK_URL"), os.Getenv("AUTHENTIK_TOKEN")
	if url == "" || token == "" {
		t.Skip("set AUTHENTIK_URL and AUTHENTIK_TOKEN to run integration tests")
	}

	client, err := authentik.New(authentik.Config{
		BaseURL: url,
		Token:   token,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}
	return client
}

func TestIntegrationVersionAndCompatibility(t *testing.T) {
	client := liveClient(t)
	ctx := context.Background()

	version, err := client.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	t.Logf("authentik reported version %s", version)

	compat, err := client.Compatibility(ctx)
	if err != nil {
		t.Fatalf("Compatibility: %v", err)
	}
	t.Logf("compatibility: raw=%q supported=%v message=%q", compat.Raw, compat.Supported(), compat.Message)

	if compat.Raw == "" {
		t.Error("expected a non-empty reported version")
	}
	// The local stack runs a version from supported-versions.yaml, so the gate
	// must accept it. If this fails, the gate and the test matrix disagree.
	if !compat.Supported() {
		t.Errorf("gate rejected %q, which the test stack is pinned to: %s", compat.Raw, compat.Message)
	}
}

// Every authentik install ships default flows, so resolving one proves the
// resolver works against real API shapes rather than against a fixture.
func TestIntegrationResolveFlow(t *testing.T) {
	client := liveClient(t)
	ctx := context.Background()

	flows, _, err := client.API().FlowsAPI.FlowsInstancesList(ctx).PageSize(100).Execute()
	if err != nil {
		t.Fatalf("listing flows: %v", err)
	}
	if len(flows.Results) == 0 {
		t.Skip("instance has no flows to resolve")
	}

	want := flows.Results[0]
	got, err := client.ResolveFlow(ctx, want.Slug)
	if err != nil {
		t.Fatalf("ResolveFlow(%q): %v", want.Slug, err)
	}
	if got != want.Pk {
		t.Errorf("ResolveFlow(%q) = %q, want %q", want.Slug, got, want.Pk)
	}
	t.Logf("resolved flow %q to %s", want.Slug, got)

	// A UUID must pass through untouched, so users may paste either form.
	passthrough, err := client.ResolveFlow(ctx, want.Pk)
	if err != nil {
		t.Fatalf("ResolveFlow(uuid): %v", err)
	}
	if passthrough != want.Pk {
		t.Errorf("ResolveFlow(uuid) = %q, want it returned unchanged", passthrough)
	}
}

func TestIntegrationResolveMissingFlowIsNotFound(t *testing.T) {
	client := liveClient(t)

	_, err := client.ResolveFlow(context.Background(), "definitely-not-a-real-flow-slug-9f3a")
	if err == nil {
		t.Fatal("expected an error for a slug that does not exist")
	}
	// Controllers branch on this to decide requeue-vs-fail, so the exact
	// classification matters more than the message.
	if !authentik.IsNotFound(err) {
		t.Errorf("expected IsNotFound, got %v", err)
	}
}

func TestIntegrationResolvePropertyMapping(t *testing.T) {
	client := liveClient(t)
	ctx := context.Background()

	mappings, _, err := client.API().PropertymappingsAPI.PropertymappingsAllList(ctx).PageSize(100).Execute()
	if err != nil {
		t.Fatalf("listing property mappings: %v", err)
	}
	if len(mappings.Results) == 0 {
		t.Skip("instance has no property mappings to resolve")
	}

	want := mappings.Results[0]
	got, err := client.ResolvePropertyMapping(ctx, want.Name)
	if err != nil {
		t.Fatalf("ResolvePropertyMapping(%q): %v", want.Name, err)
	}
	if got != want.Pk {
		t.Errorf("ResolvePropertyMapping(%q) = %q, want %q", want.Name, got, want.Pk)
	}
	t.Logf("resolved property mapping %q to %s", want.Name, got)
}

// A bad token must classify as Unauthorized rather than as a generic failure,
// so a connection with a stale credential reports something actionable.
func TestIntegrationBadTokenIsUnauthorized(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	url := os.Getenv("AUTHENTIK_URL")
	if url == "" {
		t.Skip("set AUTHENTIK_URL to run integration tests")
	}

	client, err := authentik.New(authentik.Config{
		BaseURL: url,
		Token:   "not-a-valid-token",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("building client: %v", err)
	}

	_, err = client.Version(context.Background())
	if err == nil {
		t.Fatal("expected an error when using an invalid token")
	}
	if !authentik.IsUnauthorized(err) {
		t.Errorf("expected IsUnauthorized, got %v", err)
	}
}
