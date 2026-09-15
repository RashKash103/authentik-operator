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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// fenceStart matches an opening code fence, including the attribute-list form
// the docs use for annotated blocks: ```{ .yaml .annotate title="x.yaml" }
var (
	fenceStart      = regexp.MustCompile("^```+\\s*(\\{[^}]*\\}|[a-zA-Z]*)\\s*$")
	fenceEnd        = regexp.MustCompile("^```+\\s*$")
	annotationMarks = regexp.MustCompile(`(?m)\s*#\s*\(\d+\)!\s*$`)
)

// TestDocumentedManifestsAreValid applies every manifest in the documentation
// to a real API server.
//
// Documentation examples rot in total silence. Nothing compiles them, nothing
// applies them, and a field renamed in the CRD leaves every page confidently
// wrong — which is worse than no page at all, because a reader trusts it and
// spends their afternoon on a manifest that could never have worked. This has
// already happened here once, across most of the guides at the same time.
func TestDocumentedManifestsAreValid(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "docs")

	files, err := documentationFiles()
	if err != nil {
		t.Fatalf("listing documentation: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no documentation files found; the paths in this test are wrong")
	}

	applied := 0
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}

		for _, block := range yamlBlocks(string(raw)) {
			for i, doc := range splitYAML(block.body) {
				obj := &unstructured.Unstructured{}
				if err := yaml.Unmarshal([]byte(doc), obj); err != nil {
					t.Errorf("%s:%d document %d: not valid YAML: %v",
						relativeToRepo(path), block.line, i, err)
					continue
				}
				// Only this operator's kinds: a block may show a Deployment,
				// a Flux HelmRelease or a kustomization, whose CRDs this
				// cluster does not have.
				if !strings.HasPrefix(obj.GetAPIVersion(), "authentik.k8s.rka.sh/") {
					continue
				}
				// A fragment showing one field, not a whole object. Those are
				// legitimate in prose and there is nothing to apply.
				if obj.GetName() == "" {
					continue
				}

				// Every namespaced kind goes into the test's own namespace;
				// pages name one for readability and it need not exist.
				if obj.GetKind() != "ClusterAuthentikConnection" {
					obj.SetNamespace(ns)
				}

				// A server-side dry run: strict, because that is what
				// `kubectl apply` does for the reader copying the block, and
				// dry run because the same object name recurs across pages and
				// only the schema and its CEL rules are under test.
				if err := c.Create(ctx, obj,
					client.FieldValidation("Strict"), client.DryRunAll); err != nil {
					t.Errorf("%s:%d (%s/%s) was rejected: %v",
						relativeToRepo(path), block.line, obj.GetKind(), obj.GetName(), err)
					continue
				}
				applied++
			}
		}
	}

	if applied == 0 {
		t.Fatal("no documented manifests were applied; the extractor is not finding them")
	}
	t.Logf("validated %d documented manifests across %d files", applied, len(files))
}

// documentationFiles returns every markdown file that can carry a manifest.
func documentationFiles() ([]string, error) {
	root := filepath.Join("..", "..")
	var out []string

	for _, dir := range []string{"docs", "examples"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".md") {
				out = append(out, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", dir, err)
		}
	}
	out = append(out, filepath.Join(root, "README.md"))
	return out, nil
}

type yamlBlock struct {
	// line is the 1-indexed line of the block's first content line, so a
	// failure points at somewhere an editor can jump to.
	line int
	body string
}

// yamlBlocks extracts fenced YAML from markdown.
func yamlBlocks(text string) []yamlBlock {
	var out []yamlBlock
	lines := strings.Split(text, "\n")

	for i := 0; i < len(lines); i++ {
		m := fenceStart.FindStringSubmatch(lines[i])
		if m == nil || !strings.Contains(m[1], "yaml") {
			continue
		}
		start := i + 1
		j := start
		for j < len(lines) && !fenceEnd.MatchString(lines[j]) {
			j++
		}
		body := strings.Join(lines[start:j], "\n")
		i = j

		// Annotated blocks carry "# (1)!" markers that pair with a numbered
		// list below. They are documentation syntax, not YAML comments.
		body = annotationMarks.ReplaceAllString(body, "")
		if strings.Contains(body, "authentik.k8s.rka.sh") {
			out = append(out, yamlBlock{line: start + 1, body: body})
		}
	}
	return out
}

// relativeToRepo trims the "../.." this test runs under, so failures print a
// path that can be pasted into an editor.
func relativeToRepo(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), "../../")
}
