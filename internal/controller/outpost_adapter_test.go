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
	"slices"
	"strings"
	"testing"

	api "goauthentik.io/api/v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// newOutpost builds an Outpost with the fields these tests care about.
func newOutpost(refs ...authentikv1alpha1.ProviderReference) *authentikv1alpha1.Outpost {
	outpost := &authentikv1alpha1.Outpost{
		Spec: authentikv1alpha1.OutpostSpec{
			Type:         authentikv1alpha1.OutpostTypeProxy,
			ProviderRefs: refs,
		},
	}
	outpost.Name = "edge"
	outpost.Namespace = "team-a"
	return outpost
}

func TestOutpostRequestCarriesResolvedReferences(t *testing.T) {
	outpost := newOutpost()
	outpost.Spec.Config = map[string]apiextensionsv1.JSON{
		"log_level":           {Raw: []byte(`"debug"`)},
		"kubernetes_replicas": {Raw: []byte(`2`)},
	}

	adapter := &outpostAdapter{
		outpost:           outpost,
		providerIDs:       []int32{7, 9},
		serviceConnection: "sc-uuid",
	}

	req, err := adapter.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	if req.GetName() != "edge" {
		t.Errorf("name = %q, want the resource name", req.GetName())
	}
	if req.GetType() != api.OUTPOSTTYPEENUM_PROXY {
		t.Errorf("type = %q, want proxy", req.GetType())
	}
	if !slices.Equal(req.GetProviders(), []int32{7, 9}) {
		t.Errorf("providers = %v, want the resolved primary keys in spec order", req.GetProviders())
	}
	if req.GetServiceConnection() != "sc-uuid" {
		t.Errorf("serviceConnection = %q, want the resolved UUID", req.GetServiceConnection())
	}
	if req.GetConfig()["log_level"] != "debug" {
		t.Errorf("config = %v, want the decoded free-form values", req.GetConfig())
	}
	// JSON numbers decode as float64; the value has to survive the round trip
	// rather than being dropped for not being a string.
	if got, ok := req.GetConfig()["kubernetes_replicas"].(float64); !ok || got != 2 {
		t.Errorf("config kubernetes_replicas = %v, want 2", req.GetConfig()["kubernetes_replicas"])
	}
}

// authentik requires the providers field, and an outpost that genuinely serves
// nothing still has to send an empty list rather than null.
func TestOutpostRequestSendsEmptyProviderList(t *testing.T) {
	adapter := &outpostAdapter{outpost: newOutpost()}

	req, err := adapter.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if req.Providers == nil {
		t.Fatal("providers must be an empty list, never null")
	}
	if len(req.Providers) != 0 {
		t.Errorf("providers = %v, want empty", req.Providers)
	}
}

// Dropping serviceConnectionRef from the spec has to detach the connection.
// Omitting the field from the request would leave authentik managing a
// deployment nobody asked for any more, while the resource reported success.
func TestOutpostRequestClearsRemovedServiceConnection(t *testing.T) {
	adapter := &outpostAdapter{outpost: newOutpost()}

	req, err := adapter.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if !req.ServiceConnection.IsSet() || req.ServiceConnection.Get() != nil {
		t.Error("serviceConnection must be sent as an explicit null so authentik detaches it")
	}
}

// A config value that will not decode fails identically on every retry, so it
// is reported as a rejected spec and the resource stops spinning.
func TestOutpostConfigRejectsUndecodableValue(t *testing.T) {
	_, err := outpostConfig(map[string]apiextensionsv1.JSON{
		"broken": {Raw: []byte(`{"unterminated":`)},
	})
	if err == nil {
		t.Fatal("expected an error for a value that is not valid JSON")
	}
	if !authentik.IsValidation(err) {
		t.Errorf("err = %v, want a validation error so ResultFor stops retrying", err)
	}
	if reason, retry := ResultFor(err); reason != authentikv1alpha1.ReasonInvalidSpec || retry {
		t.Errorf("ResultFor = (%s, %v), want (InvalidSpec, false)", reason, retry)
	}
}

// The offending key is named so the user can find it, but never its value:
// an administrator may legitimately place a secret in the outpost config.
func TestOutpostConfigErrorNamesKeyNotValue(t *testing.T) {
	_, err := outpostConfig(map[string]apiextensionsv1.JSON{
		"docker_labels": {Raw: []byte(`{"token": "hunter2"`)},
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "docker_labels") {
		t.Errorf("err = %q, want the offending key named", msg)
	}
	if strings.Contains(msg, "hunter2") {
		t.Fatalf("err leaked the config value: %q", msg)
	}
}

func TestOutpostEquivalence(t *testing.T) {
	base := func() *api.Outpost {
		out := &api.Outpost{
			Name:      "edge",
			Type:      api.OUTPOSTTYPEENUM_PROXY,
			Providers: []int32{1, 2},
			Config:    map[string]any{"log_level": "info"},
		}
		out.SetServiceConnection("sc-uuid")
		return out
	}

	cases := []struct {
		name   string
		mutate func(*api.Outpost)
		want   bool
	}{
		{"identical", func(*api.Outpost) {}, true},
		{"renamed", func(o *api.Outpost) { o.Name = "other" }, false},
		{"retyped", func(o *api.Outpost) { o.Type = api.OUTPOSTTYPEENUM_LDAP }, false},
		{"provider added", func(o *api.Outpost) { o.Providers = []int32{1, 2, 3} }, false},
		// Order is meaningful only in the sense that authentik echoes back what
		// it was sent; a reordered list is still a different payload and is
		// cheap to correct, so it counts as drift.
		{"provider reordered", func(o *api.Outpost) { o.Providers = []int32{2, 1} }, false},
		{"service connection detached", func(o *api.Outpost) { o.SetServiceConnectionNil() }, false},
		{"config changed", func(o *api.Outpost) { o.Config = map[string]any{"log_level": "debug"} }, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := base(), base()
			tc.mutate(right)
			if got := outpostEquivalent(left, right); got != tc.want {
				t.Errorf("outpostEquivalent = %v, want %v", got, tc.want)
			}
		})
	}
}

// authentik round-trips the config through JSON, so a map that came back from
// the server holds float64 where the spec held an integer. Comparing the maps
// structurally would report drift on every single reconcile.
func TestConfigEquivalenceIgnoresNumericRepresentation(t *testing.T) {
	spec := map[string]any{"kubernetes_replicas": 2}
	observed := map[string]any{"kubernetes_replicas": float64(2)}

	if !configEquivalent(spec, observed) {
		t.Error("an integer and the float64 authentik echoes back must compare equal")
	}
}
