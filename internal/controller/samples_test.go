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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestSamplesAreValid applies every file in config/samples to a real API
// server.
//
// Samples are the first thing a new user copies. One that the API server
// rejects is worse than no sample at all, and nothing else in the build would
// notice: they are never compiled, linted or applied.
func TestSamplesAreValid(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "samples")

	dir := filepath.Join("..", "..", "config", "samples")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading samples: %v", err)
	}

	applied := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") || name == "kustomization.yaml" {
			continue
		}

		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}

			for i, doc := range splitYAML(string(raw)) {
				obj := &unstructured.Unstructured{}
				if err := yaml.Unmarshal([]byte(doc), obj); err != nil {
					t.Fatalf("%s document %d: parsing: %v", name, i, err)
				}
				if obj.GetKind() == "" {
					continue
				}

				// Samples name a namespace for readability; redirect them into
				// the test's own so repeated runs cannot collide.
				if obj.GetNamespace() != "" {
					obj.SetNamespace(ns)
				}

				// Strict field validation, because that is what `kubectl
				// apply` does. The default prunes an unknown field silently,
				// which let a sample naming a field that no longer exists pass
				// this test while failing for every user who copied it.
				if err := c.Create(ctx, obj, client.FieldValidation("Strict")); err != nil {
					t.Errorf("%s document %d (%s/%s) was rejected: %v",
						name, i, obj.GetKind(), obj.GetName(), err)
					continue
				}
				applied++

				// Clean up so a cluster-scoped sample does not collide with
				// the next test file.
				t.Cleanup(func() {
					_ = c.Delete(context.Background(), obj, client.PropagationPolicy("Background"))
				})
			}
		})
	}

	if applied == 0 {
		t.Fatal("no sample objects were applied; the samples directory looks empty")
	}
	t.Logf("validated %d sample objects", applied)
}

// TestExampleManifestsAreValid applies every authentik resource under
// examples/ to a real API server.
//
// Examples rot silently: nothing compiles them, and a field renamed in the CRD
// leaves them quietly wrong. Walking the whole directory rather than naming one
// subdirectory means a new example cannot be added outside the check -- which
// is what happened when the Flux examples were the only ones covered.
func TestExampleManifestsAreValid(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "examples")

	root := filepath.Join("..", "..", "examples")
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		// A kustomization is a build input, not a manifest; kustomize's own
		// kinds are not in this cluster.
		if filepath.Base(path) == "kustomization.yaml" {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking examples: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no example manifests found; this test is looking in the wrong place")
	}

	applied := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), "../../")

		for i, doc := range splitYAML(string(raw)) {
			obj := &unstructured.Unstructured{}
			if err := yaml.Unmarshal([]byte(doc), obj); err != nil {
				t.Errorf("%s document %d: parsing: %v", rel, i, err)
				continue
			}
			// Skip Flux kinds and the Secret template, which is deliberately
			// not a usable manifest.
			if !strings.HasPrefix(obj.GetAPIVersion(), "authentik.k8s.rka.sh/") {
				continue
			}

			obj.SetNamespace(ns)
			// Strict, because that is what `kubectl apply` does for the reader
			// copying it, and dry run because the same name recurs across
			// examples while only the schema is under test.
			if err := c.Create(ctx, obj,
				client.FieldValidation("Strict"), client.DryRunAll); err != nil {
				t.Errorf("%s document %d (%s/%s) was rejected: %v",
					rel, i, obj.GetKind(), obj.GetName(), err)
				continue
			}
			applied++
		}
	}

	if applied == 0 {
		t.Fatal("no authentik resources found in the examples")
	}
	t.Logf("validated %d example objects across %d files", applied, len(files))
}

// splitYAML splits a multi-document YAML file on its document separators.
func splitYAML(content string) []string {
	parts := strings.Split(content, "\n---")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(stripComments(part)) != "" {
			out = append(out, part)
		}
	}
	return out
}

// stripComments removes comment-only lines so a file of pure commentary is not
// mistaken for a document.
func stripComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}
