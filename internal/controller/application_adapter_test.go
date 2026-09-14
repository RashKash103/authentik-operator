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
	"encoding/json"
	"slices"
	"strings"
	"testing"

	api "goauthentik.io/api/v3"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

func ptrBool(v bool) *bool { return &v }

// The slug, not the display name, is what authentik keys an application by,
// so it is what a pre-existing application collides on and therefore what the
// engine must use for adoption checks.
func TestApplicationAdapterDesiredNameIsTheSlug(t *testing.T) {
	app := newTestApplication()
	app.Spec.Name = "Grafana Dashboards"
	app.Spec.Slug = "grafana"

	adapter := &applicationAdapter{app: app}
	if got := adapter.DesiredName(); got != "grafana" {
		t.Errorf("DesiredName() = %q, want the slug %q", got, "grafana")
	}
}

// The display name defaults to the resource name, but the request always
// carries the slug verbatim: it is immutable and is the object's identity.
func TestApplicationBuildRequestNameAndSlug(t *testing.T) {
	tests := []struct {
		name     string
		specName string
		wantName string
	}{
		{
			name:     "an unset name falls back to the resource name",
			specName: "",
			wantName: "grafana",
		},
		{
			name:     "an explicit name wins",
			specName: "Grafana Dashboards",
			wantName: "Grafana Dashboards",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApplication()
			app.Spec.Name = tc.specName

			req := (&applicationAdapter{app: app}).buildRequest()
			if req.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", req.Name, tc.wantName)
			}
			if req.Slug != "grafana" {
				t.Errorf("Slug = %q, want %q", req.Slug, "grafana")
			}
		})
	}
}

// Detaching a provider has to be expressed as an explicit null, not an omitted
// field: authentik leaves an omitted field alone, so removing providerRef from
// the spec would otherwise leave the old provider attached forever.
func TestApplicationBuildRequestDetachesProviderExplicitly(t *testing.T) {
	tests := []struct {
		name         string
		providerID   *int32
		backchannel  []int32
		wantProvider string
		wantBackchan string
	}{
		{
			name:         "no provider sends an explicit null",
			providerID:   nil,
			backchannel:  nil,
			wantProvider: `"provider":null`,
			wantBackchan: `"backchannel_providers":[]`,
		},
		{
			name:         "a resolved provider sends its primary key",
			providerID:   ptr32(7),
			backchannel:  []int32{9, 11},
			wantProvider: `"provider":7`,
			wantBackchan: `"backchannel_providers":[9,11]`,
		},
		{
			// Emptying the list must also be sent, for the same reason as the
			// null above: an omitted list would not detach anything.
			name:         "an emptied back-channel list is still sent",
			providerID:   ptr32(7),
			backchannel:  []int32{},
			wantProvider: `"provider":7`,
			wantBackchan: `"backchannel_providers":[]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &applicationAdapter{
				app:                    newTestApplication(),
				providerID:             tc.providerID,
				backchannelProviderIDs: tc.backchannel,
			}

			body, err := json.Marshal(adapter.buildRequest())
			if err != nil {
				t.Fatalf("marshalling request: %v", err)
			}
			if !strings.Contains(string(body), tc.wantProvider) {
				t.Errorf("body %s does not contain %s", body, tc.wantProvider)
			}
			if !strings.Contains(string(body), tc.wantBackchan) {
				t.Errorf("body %s does not contain %s", body, tc.wantBackchan)
			}
		})
	}
}

func TestApplicationBuildRequestMetadata(t *testing.T) {
	app := newTestApplication()
	app.Spec.MetaLaunchURL = "https://grafana.example.com"
	app.Spec.MetaIcon = "https://cdn.example.com/grafana.png"
	app.Spec.MetaDescription = "Dashboards"
	app.Spec.MetaPublisher = "Platform"
	app.Spec.Group = "Observability"
	app.Spec.PolicyEngineMode = "all"
	app.Spec.OpenInNewTab = ptrBool(true)
	app.Spec.MetaHide = ptrBool(true)

	req := (&applicationAdapter{app: app}).buildRequest()

	if got := req.GetMetaLaunchUrl(); got != "https://grafana.example.com" {
		t.Errorf("MetaLaunchUrl = %q", got)
	}
	if got := req.GetMetaIcon(); got != "https://cdn.example.com/grafana.png" {
		t.Errorf("MetaIcon = %q", got)
	}
	if got := req.GetMetaDescription(); got != "Dashboards" {
		t.Errorf("MetaDescription = %q", got)
	}
	if got := req.GetMetaPublisher(); got != "Platform" {
		t.Errorf("MetaPublisher = %q", got)
	}
	if got := req.GetGroup(); got != "Observability" {
		t.Errorf("Group = %q", got)
	}
	if got := req.GetPolicyEngineMode(); got != api.PolicyEngineMode("all") {
		t.Errorf("PolicyEngineMode = %q", got)
	}
	if !req.GetOpenInNewTab() || !req.GetMetaHide() {
		t.Error("openInNewTab and metaHide should both be true")
	}
}

// An unset optional field must stay off the request entirely. Sending a zero
// value instead would overwrite whatever an administrator configured in the
// authentik UI for a field this resource does not manage.
func TestApplicationBuildRequestOmitsUnsetOptionalFields(t *testing.T) {
	app := newTestApplication()
	app.Spec.PolicyEngineMode = ""

	body, err := json.Marshal((&applicationAdapter{app: app}).buildRequest())
	if err != nil {
		t.Fatalf("marshalling request: %v", err)
	}

	for _, field := range []string{
		"meta_launch_url", "meta_icon", "meta_description",
		"meta_publisher", "group", "policy_engine_mode",
		"open_in_new_tab", "meta_hide",
	} {
		if strings.Contains(string(body), field) {
			t.Errorf("body %s should not contain unset field %q", body, field)
		}
	}
}

func TestApplicationEquivalent(t *testing.T) {
	base := func() *api.Application {
		app := api.NewApplication("pk", "uuid", "Grafana", "grafana",
			*api.NewNullableProvider(nil), nil,
			*api.NewNullableString(nil), *api.NewNullableString(nil),
			*api.NewNullableThemedUrls(nil))
		app.SetProvider(7)
		app.SetBackchannelProviders([]int32{9})
		app.SetGroup("Observability")
		return app
	}

	tests := []struct {
		name   string
		mutate func(*api.Application)
		want   bool
	}{
		{
			name:   "identical states are equivalent",
			mutate: func(*api.Application) {},
			want:   true,
		},
		{
			// authentik computes launch_url from the provider. Treating it as
			// drift would make every reconcile issue a pointless update and
			// emit a DriftCorrected event forever.
			name:   "a computed launch URL is not drift",
			mutate: func(a *api.Application) { a.SetLaunchUrl("https://grafana.example.com") },
			want:   true,
		},
		{
			name:   "a changed display name is drift",
			mutate: func(a *api.Application) { a.Name = "Renamed" },
			want:   false,
		},
		{
			name:   "a changed provider is drift",
			mutate: func(a *api.Application) { a.SetProvider(8) },
			want:   false,
		},
		{
			name:   "a detached provider is drift",
			mutate: func(a *api.Application) { a.SetProviderNil() },
			want:   false,
		},
		{
			// The list is ordered in authentik, so a reorder is a real change.
			name:   "a reordered back-channel list is drift",
			mutate: func(a *api.Application) { a.SetBackchannelProviders([]int32{9, 11}) },
			want:   false,
		},
		{
			name:   "a changed group is drift",
			mutate: func(a *api.Application) { a.SetGroup("Other") },
			want:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			left, right := base(), base()
			tc.mutate(right)

			if got := applicationEquivalent(left, right); got != tc.want {
				t.Errorf("applicationEquivalent = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil observed state means "nothing to compare against", which the adapter
// reports as changed so the first reconcile after a restart still records the
// update rather than claiming a clean no-op it did not verify.
func TestApplicationEquivalentHandlesNil(t *testing.T) {
	if !applicationEquivalent(nil, nil) {
		t.Error("two nil states should compare equal")
	}
	if applicationEquivalent(nil, &api.Application{}) {
		t.Error("nil and non-nil states should not compare equal")
	}
}

// ProviderRefs is what the field index is built from, so it has to return the
// primary reference as well as the back-channel ones, in a stable order.
func TestApplicationProviderRefs(t *testing.T) {
	app := newTestApplication(
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "primary"},
		authentikv1alpha1.ProviderReference{Kind: authentikv1alpha1.ProviderKindOAuth2, Name: "scim"},
	)

	got := make([]string, 0, 2)
	for _, ref := range app.ProviderRefs() {
		got = append(got, ref.Name)
	}
	if want := []string{"primary", "scim"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
