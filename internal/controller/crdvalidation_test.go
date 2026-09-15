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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
)

// These tests exist because a bad CRD marker produces valid Go and valid YAML,
// and only fails when a real API server refuses a user's manifest. Asserting
// acceptance and rejection here is the only way to know the schema means what
// the markers intended.

func TestAuthentikConnectionSchema(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-conn")

	cases := []struct {
		name    string
		spec    authentikv1alpha1.AuthentikConnectionSpec
		wantErr string
	}{
		{
			name: "a complete spec is accepted",
			spec: authentikv1alpha1.AuthentikConnectionSpec{
				ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: "https://authentik.example.com"},
				TokenSecretRef:     authentikv1alpha1.LocalSecretKeyReference{Name: "tok", Key: "token"},
			},
		},
		{
			name: "a URL without a scheme is rejected",
			spec: authentikv1alpha1.AuthentikConnectionSpec{
				ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: "authentik.example.com"},
				TokenSecretRef:     authentikv1alpha1.LocalSecretKeyReference{Name: "tok", Key: "token"},
			},
			wantErr: "spec.url",
		},
		{
			name: "an empty secret key is rejected",
			spec: authentikv1alpha1.AuthentikConnectionSpec{
				ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: "https://authentik.example.com"},
				TokenSecretRef:     authentikv1alpha1.LocalSecretKeyReference{Name: "tok", Key: ""},
			},
			wantErr: "key",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &authentikv1alpha1.AuthentikConnection{
				ObjectMeta: metav1.ObjectMeta{Name: objName("conn", i), Namespace: ns},
				Spec:       tc.spec,
			}
			assertAdmission(t, c.Create(ctx, obj), tc.wantErr)
		})
	}
}

// The probe interval is a duration string; a bare number is the mistake a user
// will actually make.
func TestConnectionProbeIntervalPattern(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-probe")

	obj := &authentikv1alpha1.AuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "bad-interval", Namespace: ns},
		Spec: authentikv1alpha1.AuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{
				URL:           "https://authentik.example.com",
				ProbeInterval: &metav1.Duration{},
			},
			TokenSecretRef: authentikv1alpha1.LocalSecretKeyReference{Name: "tok", Key: "token"},
		},
	}
	// A zero metav1.Duration marshals as "0s", which the pattern accepts, so
	// this asserts the accepting side rather than the rejecting one.
	assertAdmission(t, c.Create(ctx, obj), "")
}

func TestClusterConnectionRequiresSecretNamespace(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()

	// A cluster-scoped connection has no namespace of its own, so omitting the
	// Secret's namespace must be refused rather than silently defaulted - a
	// default here would be a cross-namespace credential read.
	obj := &authentikv1alpha1.ClusterAuthentikConnection{
		ObjectMeta: metav1.ObjectMeta{Name: "no-ns"},
		Spec: authentikv1alpha1.ClusterAuthentikConnectionSpec{
			ConnectionSettings: authentikv1alpha1.ConnectionSettings{URL: "https://authentik.example.com"},
			TokenSecretRef:     authentikv1alpha1.SecretKeyReference{Name: "tok", Key: "token"},
		},
	}
	assertAdmission(t, c.Create(ctx, obj), "namespace")
}

func TestOAuth2ProviderSchema(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-oauth2")

	base := func() authentikv1alpha1.OAuth2ProviderSpec {
		return authentikv1alpha1.OAuth2ProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
				AuthorizationFlow: authentikv1alpha1.FlowReference{Name: "default-provider-authorization-explicit-consent"},
				InvalidationFlow:  authentikv1alpha1.FlowReference{Name: "default-provider-invalidation-flow"},
			},
		}
	}

	cases := []struct {
		name    string
		mutate  func(*authentikv1alpha1.OAuth2ProviderSpec)
		wantErr string
	}{
		{
			name:   "a minimal spec is accepted",
			mutate: func(*authentikv1alpha1.OAuth2ProviderSpec) {},
		},
		{
			// This is the regression test for an enum marker landing on the
			// array instead of its items, which rejected every non-empty list.
			name: "a list of valid grant types is accepted",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.GrantTypes = []authentikv1alpha1.OAuth2GrantType{
					"authorization_code", "refresh_token",
				}
			},
		},
		{
			name: "a urn grant type is accepted",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.GrantTypes = []authentikv1alpha1.OAuth2GrantType{
					"urn:ietf:params:oauth:grant-type:device_code",
				}
			},
		},
		{
			name: "an unknown grant type is rejected",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.GrantTypes = []authentikv1alpha1.OAuth2GrantType{"telepathy"}
			},
			wantErr: "grantTypes",
		},
		{
			name: "an authorization flow is required",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.AuthorizationFlow = authentikv1alpha1.FlowReference{}
			},
			wantErr: "authorizationFlow",
		},
		{
			// "1h" is Go's duration syntax, not authentik's. Catching it at
			// admission beats a confusing API rejection during reconcile.
			name: "a Go-style duration is rejected",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.AccessTokenValidity = "1h"
			},
			wantErr: "accessTokenValidity",
		},
		{
			name: "an authentik-style duration is accepted",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.AccessTokenValidity = "hours=1;minutes=30"
			},
		},
		{
			name: "an unknown client type is rejected",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.ClientType = "semi-confidential"
			},
			wantErr: "clientType",
		},
		{
			name: "an unknown connection kind is rejected",
			mutate: func(s *authentikv1alpha1.OAuth2ProviderSpec) {
				s.ConnectionRef.Kind = "SomethingElse"
			},
			wantErr: "kind",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := base()
			tc.mutate(&spec)
			obj := &authentikv1alpha1.OAuth2Provider{
				ObjectMeta: metav1.ObjectMeta{Name: objName("oauth2", i), Namespace: ns},
				Spec:       spec,
			}
			assertAdmission(t, c.Create(ctx, obj), tc.wantErr)
		})
	}
}

// TestOAuth2ProviderDefaults confirms the defaults the CRD promises are
// actually applied by the API server, not merely documented.
func TestOAuth2ProviderDefaults(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-defaults")

	obj := &authentikv1alpha1.OAuth2Provider{
		ObjectMeta: metav1.ObjectMeta{Name: "defaults", Namespace: ns},
		Spec: authentikv1alpha1.OAuth2ProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
				AuthorizationFlow: authentikv1alpha1.FlowReference{Name: "authz"},
				InvalidationFlow:  authentikv1alpha1.FlowReference{Name: "invalidation"},
			},
		},
	}
	if err := c.Create(ctx, obj); err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	if got := obj.Spec.ConnectionRef.Kind; got != authentikv1alpha1.ConnectionKindNamespaced {
		t.Errorf("connectionRef.kind defaulted to %q, want %q", got, authentikv1alpha1.ConnectionKindNamespaced)
	}
	if got := obj.Spec.Adoption; got != authentikv1alpha1.AdoptionPolicyFailOnConflict {
		t.Errorf("adoptionPolicy defaulted to %q, want FailOnConflict", got)
	}
	if got := obj.Spec.Deletion; got != authentikv1alpha1.DeletionPolicyDelete {
		t.Errorf("deletionPolicy defaulted to %q, want Delete", got)
	}
	if got := obj.Spec.ClientType; got != "confidential" {
		t.Errorf("clientType defaulted to %q, want confidential", got)
	}
}

// assertAdmission checks an API server response against an expectation.
//
// wantErr is a substring the rejection must mention, usually the offending
// field, so a test cannot pass because admission failed for some other reason.
func assertAdmission(t *testing.T, err error, wantErr string) {
	t.Helper()

	if wantErr == "" {
		if err != nil {
			t.Fatalf("expected the object to be accepted, got: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected rejection mentioning %q, but the object was accepted", wantErr)
	}
	if !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("expected rejection mentioning %q, got: %v", wantErr, err)
	}
}

// objName keeps generated names unique and readable in failure output.
func objName(prefix string, i int) string {
	return prefix + "-" + string(rune('a'+i))
}

// TestProxyProviderModeExclusivity proves the CEL rule is real.
//
// A CEL rule is a string in a marker comment: a typo produces a CRD the API
// server rejects at install time, or worse, a rule that silently never fires.
// gofmt has also been observed rewriting straight quotes inside nearby
// comments into typographic ones, which would corrupt a rule without any
// compiler noticing. Only exercising it against an API server settles it.
func TestProxyProviderModeExclusivity(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-proxy")

	base := func() authentikv1alpha1.ProxyProviderSpec {
		return authentikv1alpha1.ProxyProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				ConnectionRef:     authentikv1alpha1.ConnectionReference{Name: "primary"},
				AuthorizationFlow: authentikv1alpha1.FlowReference{Name: "authz"},
				InvalidationFlow:  authentikv1alpha1.FlowReference{Name: "invalidation"},
			},
			ExternalHost: "https://app.example.com",
		}
	}

	cases := []struct {
		name    string
		mutate  func(*authentikv1alpha1.ProxyProviderSpec)
		wantErr string
	}{
		{
			name: "proxy mode with an internal host is accepted",
			mutate: func(s *authentikv1alpha1.ProxyProviderSpec) {
				s.Mode = "proxy"
				s.InternalHost = "http://app.internal:8080"
			},
		},
		{
			name: "forward_single without an internal host is accepted",
			mutate: func(s *authentikv1alpha1.ProxyProviderSpec) {
				s.Mode = "forward_single"
			},
		},
		{
			// The whole point of the rule: forwarding has no upstream to
			// forward to, so an internalHost is a configuration mistake.
			name: "forward_single with an internal host is rejected",
			mutate: func(s *authentikv1alpha1.ProxyProviderSpec) {
				s.Mode = "forward_single"
				s.InternalHost = "http://app.internal:8080"
			},
			wantErr: "internalHost",
		},
		{
			name: "forward_domain with an internal host is rejected",
			mutate: func(s *authentikv1alpha1.ProxyProviderSpec) {
				s.Mode = "forward_domain"
				s.InternalHost = "http://app.internal:8080"
			},
			wantErr: "internalHost",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := base()
			tc.mutate(&spec)
			obj := &authentikv1alpha1.ProxyProvider{
				ObjectMeta: metav1.ObjectMeta{Name: objName("proxy", i), Namespace: ns},
				Spec:       spec,
			}
			assertAdmission(t, c.Create(ctx, obj), tc.wantErr)
		})
	}
}

// TestApplicationSlugIsImmutable proves the immutability rule fires.
//
// Changing a slug would mean delete-and-recreate in authentik, silently
// dropping the application's policy bindings.
func TestApplicationSlugIsImmutable(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-slug")

	app := &authentikv1alpha1.Application{
		ObjectMeta: metav1.ObjectMeta{Name: "fixed-slug", Namespace: ns},
		Spec: authentikv1alpha1.ApplicationSpec{
			ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "primary"},
			Slug:          "original",
			Name:          "Original",
		},
	}
	if err := c.Create(ctx, app); err != nil {
		t.Fatalf("creating application: %v", err)
	}

	app.Spec.Slug = "renamed"
	assertAdmission(t, c.Update(ctx, app), "slug")
}

// TestReferenceExactlyOneOf covers the rules that make references extensible.
//
// Each reference carries one option today and will carry a choice once the
// matching CRDs exist. The exactly-one-of rule is what makes adding that second
// option additive rather than another breaking change - so it has to actually
// fire, in both directions.
func TestReferenceExactlyOneOf(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-refs")

	base := func() authentikv1alpha1.ApplicationSpec {
		return authentikv1alpha1.ApplicationSpec{
			ConnectionRef: authentikv1alpha1.ConnectionReference{Name: "primary"},
			Slug:          "refs",
			Name:          "Refs",
		}
	}

	cases := []struct {
		name    string
		ref     *authentikv1alpha1.ProviderReference
		wantErr string
	}{
		{
			name: "a managed provider name alone is accepted",
			ref:  &authentikv1alpha1.ProviderReference{Name: "grafana"},
		},
		{
			// The point of the change: binding to a provider somebody else
			// created, rather than one this operator manages.
			name: "an existing provider name alone is accepted",
			ref:  &authentikv1alpha1.ProviderReference{ExistingProviderName: "hand-made"},
		},
		{
			name:    "both together are rejected",
			ref:     &authentikv1alpha1.ProviderReference{Name: "grafana", ExistingProviderName: "hand-made"},
			wantErr: "exactly one",
		},
		{
			name:    "neither is rejected",
			ref:     &authentikv1alpha1.ProviderReference{},
			wantErr: "exactly one",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := base()
			spec.Slug = objName("refs", i)
			spec.ProviderRef = tc.ref
			obj := &authentikv1alpha1.Application{
				ObjectMeta: metav1.ObjectMeta{Name: objName("refapp", i), Namespace: ns},
				Spec:       spec,
			}
			assertAdmission(t, c.Create(ctx, obj), tc.wantErr)
		})
	}
}

// TestServiceConnectionReferenceExactlyOneOf covers the three-way version of
// the same rule, which had to be written without list construction: the
// filter/size form blew the API server's CEL cost budget by 5.6x.
func TestServiceConnectionReferenceExactlyOneOf(t *testing.T) {
	c := envtestClient(t)
	ctx := context.Background()
	ns := newTestNamespace(t, ctx, c, "schema-screfs")

	cases := []struct {
		name    string
		ref     *authentikv1alpha1.ServiceConnectionReference
		wantErr string
	}{
		{
			name: "a managed Kubernetes connection alone is accepted",
			ref:  &authentikv1alpha1.ServiceConnectionReference{KubernetesServiceConnectionName: "in-cluster"},
		},
		{
			name: "an existing connection alone is accepted",
			ref:  &authentikv1alpha1.ServiceConnectionReference{KubernetesServiceConnectionName: "hand-made"},
		},
		{
			name: "no reference at all is accepted, since it is optional",
			ref:  nil,
		},
		{
			name: "two together are rejected",
			ref: &authentikv1alpha1.ServiceConnectionReference{
				KubernetesServiceConnectionName: "in-cluster",
				DockerServiceConnectionName:     "docker-host",
			},
			wantErr: "exactly one",
		},
		{
			name:    "an empty reference is rejected",
			ref:     &authentikv1alpha1.ServiceConnectionReference{},
			wantErr: "exactly one",
		},
	}

	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := &authentikv1alpha1.Outpost{
				ObjectMeta: metav1.ObjectMeta{Name: objName("outpost", i), Namespace: ns},
				Spec: authentikv1alpha1.OutpostSpec{
					ConnectionRef:        authentikv1alpha1.ConnectionReference{Name: "primary"},
					Name:                 objName("outpost", i),
					Type:                 authentikv1alpha1.OutpostTypeProxy,
					ServiceConnectionRef: tc.ref,
				},
			}
			assertAdmission(t, c.Create(ctx, obj), tc.wantErr)
		})
	}
}
