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

// Package e2e exercises the operator against a real Kubernetes cluster and a
// real authentik instance.
//
// # Running these tests
//
// They are opt-in and skip by default. That is deliberate: the suite creates
// namespaces, CRDs and custom resources, and a developer's current kubecontext
// is very often a real cluster they did not intend to point a test suite at.
// Requiring an explicit opt-in means running `go test ./...` can never
// accidentally write to production.
//
//	make authentik-up
//	export E2E_ENABLED=true
//	export E2E_KUBECONTEXT=kind-authentik-operator-e2e
//	make test-e2e
package e2e

import (
	"os"
	"testing"
)

// Environment variables controlling the suite.
const (
	// envEnabled must be "true" for any test here to run.
	envEnabled = "E2E_ENABLED"
	// envKubeContext names the kubecontext to use. Required when enabled, so
	// the suite never silently inherits whatever context happens to be current.
	envKubeContext = "E2E_KUBECONTEXT"
	// envAuthentikURL is the base URL of the authentik under test.
	envAuthentikURL = "AUTHENTIK_URL"
	// envAuthentikToken is an API token for that instance.
	envAuthentikToken = "AUTHENTIK_TOKEN" //nolint:gosec // name of a variable, not a credential
)

// Config is the resolved environment for a run.
type Config struct {
	KubeContext    string
	AuthentikURL   string
	AuthentikToken string
}

// requireEnv skips the calling test unless the suite has been explicitly
// enabled and fully configured.
func requireEnv(t *testing.T) Config {
	t.Helper()

	if !enabled() {
		t.Skipf("set %s=true to run the end-to-end suite", envEnabled)
	}

	cfg := Config{
		KubeContext:    os.Getenv(envKubeContext),
		AuthentikURL:   os.Getenv(envAuthentikURL),
		AuthentikToken: os.Getenv(envAuthentikToken),
	}

	var missing []string
	for name, value := range map[string]string{
		envKubeContext:    cfg.KubeContext,
		envAuthentikURL:   cfg.AuthentikURL,
		envAuthentikToken: cfg.AuthentikToken,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		// Fail rather than skip: the operator asked for the suite to run, so
		// silently skipping would report a green run that tested nothing.
		t.Fatalf("%s=true but these are unset: %v", envEnabled, missing)
	}

	return cfg
}

// enabled reports whether the suite has been explicitly switched on.
//
// It is a separate function purely so the opt-in guard itself is testable
// without the test having to skip itself.
func enabled() bool {
	return os.Getenv(envEnabled) == "true"
}

// TestSuiteIsOptIn enforces the safety property described in the package
// comment: absent an explicit opt-in, nothing here touches a cluster.
//
// A regression that made the suite run against the ambient kubecontext would
// be genuinely dangerous, so it is asserted rather than assumed.
func TestSuiteIsOptIn(t *testing.T) {
	t.Setenv(envEnabled, "")
	if enabled() {
		t.Error("suite reported enabled with the opt-in variable unset")
	}

	t.Setenv(envEnabled, "1")
	if enabled() {
		t.Error(`only the exact value "true" should enable the suite, not "1"`)
	}

	t.Setenv(envEnabled, "true")
	if !enabled() {
		t.Error(`suite did not report enabled with the variable set to "true"`)
	}
}

// TestConnectionBecomesReady is the first real end-to-end scenario: an
// AuthentikConnection pointed at a live instance should report Ready together
// with the version it detected.
//
// TODO(EPIC 9): implement against a controller-runtime client once the E2E
// harness (cluster client, namespace fixture, cleanup) lands. Tracked
// separately so this file stays a working, compiling entry point.
func TestConnectionBecomesReady(t *testing.T) {
	cfg := requireEnv(t)
	t.Skipf("not implemented yet; would target %s via context %s", cfg.AuthentikURL, cfg.KubeContext)
}
