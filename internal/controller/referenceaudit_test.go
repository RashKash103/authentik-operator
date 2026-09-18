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
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// howEachReferenceNamesSomethingExisting records, for every reference type in
// the API, how it can name an object that already exists in authentik and is
// maintained outside the operator.
//
// Not every reference needs a field of its own. Flow, PropertyMapping and
// CertificateKeyPair references name a *resource*, and that resource carries
// existingSlug/existingName while creating nothing in authentik - so pointing
// at an existing object already costs only a small manifest. Re-adding an
// inline slug at the reference site would undo the reason those resources
// exist: one flow named once, every referrer pointing at the same object.
var howEachReferenceNamesSomethingExisting = map[string]string{
	"FlowReference":               "via the Flow resource's existingSlug",
	"PropertyMappingReference":    "via the PropertyMapping resource's existingName",
	"CertificateKeyPairReference": "via the CertificateKeyPair resource's existingName",
	"ProviderReference":           "existingProviderName",
	"ServiceConnectionReference":  "existingServiceConnectionName",
	"OutpostReference":            "via the Outpost resource's embedded/existingOutpostName",
	"ConnectionReference":         "not applicable: a connection is this operator's own credential handle, not an authentik object",
	"SecretKeyReference":          "not applicable: names a Kubernetes Secret, not an authentik object",
	"LocalSecretKeyReference":     "not applicable: names a Kubernetes Secret, not an authentik object",
}

// TestEveryReferenceCanNameAnExistingObject fails when a reference type is
// added without deciding how it names something that already exists.
//
// The gap this closes is quiet: a reference that can only name a resource the
// operator manages forces anyone with a pre-existing authentik to let the
// operator take it over, and nothing in the build would have said so. The list
// above is the decision; this test is what makes skipping it impossible.
func TestEveryReferenceCanNameAnExistingObject(t *testing.T) {
	dir := filepath.Join("..", "..", "api", "v1alpha1")
	sources, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}

	fset := token.NewFileSet()
	var found []string
	for _, path := range sources {
		base := filepath.Base(path)
		if strings.HasSuffix(base, "_test.go") || base == "zz_generated.deepcopy.go" {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || !strings.HasSuffix(spec.Name.Name, "Reference") {
				return true
			}
			if _, isStruct := spec.Type.(*ast.StructType); !isStruct {
				return true
			}
			found = append(found, spec.Name.Name)
			return true
		})
	}

	if len(found) == 0 {
		t.Fatal("no reference types found; this test is looking in the wrong place")
	}
	sort.Strings(found)

	for _, name := range found {
		if _, ok := howEachReferenceNamesSomethingExisting[name]; !ok {
			t.Errorf("%s has no recorded way to name an object that already exists in authentik.\n"+
				"Add the field, or record why it does not apply, in "+
				"howEachReferenceNamesSomethingExisting.", name)
		}
	}

	// The other direction: an entry left behind for a type that no longer
	// exists is a stale decision nobody will notice.
	for name := range howEachReferenceNamesSomethingExisting {
		if !slicesContains(found, name) {
			t.Errorf("%s is recorded here but is no longer a reference type in the API", name)
		}
	}

	t.Logf("audited %d reference types: %s", len(found), strings.Join(found, ", "))
}

func slicesContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
