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

package controller

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// testEnv is shared by every envtest-backed test; starting an API server per
// test would dominate the suite's runtime.
var (
	testEnv    *envtest.Environment
	testConfig *rest.Config
	testScheme *runtime.Scheme
)

// TestMain starts a real API server so the CRD schemas are validated by the
// thing that will actually reject a user's manifest.
//
// Unit tests cannot catch a bad schema: a marker that generates an
// unsatisfiable constraint is still valid Go and still generates valid YAML.
// Only an API server admitting or refusing an object proves the schema works.
func TestMain(m *testing.M) {
	// testing.Short() panics before the flags are parsed, and TestMain runs
	// ahead of the framework doing it.
	flag.Parse()

	if testing.Short() {
		// -short must stay Docker-free and binary-free for a fast inner loop.
		os.Exit(m.Run())
	}
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		// Without control-plane binaries these tests cannot run. Skipping is
		// handled per-test so the reason is visible in the output.
		os.Exit(m.Run())
	}

	testScheme = runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(testScheme); err != nil {
		panic(err)
	}
	if err := authentikv1alpha1.AddToScheme(testScheme); err != nil {
		panic(err)
	}

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	if err != nil {
		panic("starting envtest: " + err.Error())
	}
	testConfig = cfg

	code := m.Run()

	if err := testEnv.Stop(); err != nil {
		panic("stopping envtest: " + err.Error())
	}
	os.Exit(code)
}

// envtestClient returns a client against the test API server, skipping when
// the environment is unavailable.
func envtestClient(t *testing.T) client.Client {
	t.Helper()

	if testing.Short() {
		t.Skip("envtest does not run under -short")
	}
	if testConfig == nil {
		t.Skip("KUBEBUILDER_ASSETS is unset; run via `make test`")
	}

	c, err := client.New(testConfig, client.Options{Scheme: testScheme})
	if err != nil {
		t.Fatalf("building envtest client: %v", err)
	}
	return c
}

// newTestNamespace creates a namespace scoped to one test.
func newTestNamespace(t *testing.T, ctx context.Context, c client.Client, name string) string {
	t.Helper()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if err := c.Create(ctx, ns); err != nil {
		t.Fatalf("creating namespace %s: %v", name, err)
	}
	return name
}
