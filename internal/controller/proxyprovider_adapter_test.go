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
	"testing"

	api "goauthentik.io/api/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// newProxyProvider returns a minimally valid provider the tests can amend.
func newProxyProvider(mutate func(*authentikv1alpha1.ProxyProvider)) *authentikv1alpha1.ProxyProvider {
	p := &authentikv1alpha1.ProxyProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "default"},
		Spec: authentikv1alpha1.ProxyProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				AuthorizationFlow: "default-authz",
				InvalidationFlow:  "default-invalidation",
			},
			ExternalHost: "https://grafana.example.com",
			Mode:         authentikv1alpha1.ProxyModeProxy,
		},
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func buildProxy(t *testing.T, p *authentikv1alpha1.ProxyProvider) *api.ProxyProviderRequest {
	t.Helper()
	a := &proxyAdapter{client: newStubClient(), provider: p}
	req, err := a.buildRequest(context.Background())
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	return req
}

// The three required positional arguments are what authentik rejects the whole
// request over, so they must come from the spec.
func TestProxyBuildRequestSetsRequiredFields(t *testing.T) {
	req := buildProxy(t, newProxyProvider(nil))

	if req.Name != "grafana" {
		t.Errorf("Name = %q, want grafana", req.Name)
	}
	if req.AuthorizationFlow != "flow-authz-uuid" {
		t.Errorf("AuthorizationFlow = %q, want the resolved UUID", req.AuthorizationFlow)
	}
	if req.InvalidationFlow != "flow-invalidation-uuid" {
		t.Errorf("InvalidationFlow = %q, want the resolved UUID", req.InvalidationFlow)
	}
	if req.ExternalHost != "https://grafana.example.com" {
		t.Errorf("ExternalHost = %q", req.ExternalHost)
	}
}

// The certificate is given by key pair name and must reach authentik as a
// UUID; the name is accepted by the schema and then fails inside authentik.
func TestProxyBuildRequestResolvesCertificate(t *testing.T) {
	req := buildProxy(t, newProxyProvider(func(p *authentikv1alpha1.ProxyProvider) {
		p.Spec.Certificate = "proxy-cert"
	}))

	if req.GetCertificate() != "kp-proxy-uuid" {
		t.Errorf("certificate = %q, want the resolved UUID", req.GetCertificate())
	}
}

// internalHost is meaningless in the forwarding modes: no upstream exists for
// the outpost to forward to. The CRD refuses the combination, but a cluster
// whose schema predates that rule can still deliver one, and sending it would
// leave a field set in authentik that nothing honours.
func TestProxyBuildRequestDropsInternalHostInForwardModes(t *testing.T) {
	tests := []struct {
		name string
		mode authentikv1alpha1.ProxyMode
		want bool
	}{
		{"proxy mode forwards upstream", authentikv1alpha1.ProxyModeProxy, true},
		{"forward_single has no upstream", authentikv1alpha1.ProxyModeForwardSingle, false},
		{"forward_domain has no upstream", authentikv1alpha1.ProxyModeForwardDomain, false},
		// An empty mode means the CRD default (proxy) was applied, so the
		// upstream is still meaningful.
		{"unset mode defaults to proxy", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := buildProxy(t, newProxyProvider(func(p *authentikv1alpha1.ProxyProvider) {
				p.Spec.Mode = tt.mode
				p.Spec.InternalHost = "http://grafana.monitoring.svc:3000"
			}))

			if got := req.HasInternalHost(); got != tt.want {
				t.Errorf("internal_host sent = %v, want %v", got, tt.want)
			}
		})
	}
}

// An unset optional field must be absent rather than present as a zero value:
// sending basic_auth_enabled=false would overwrite an administrator's setting.
func TestProxyBuildRequestOmitsUnsetOptionalFields(t *testing.T) {
	req := buildProxy(t, newProxyProvider(nil))

	for _, tc := range []struct {
		field string
		set   bool
	}{
		{"internal_host", req.HasInternalHost()},
		{"internal_host_ssl_validation", req.HasInternalHostSslValidation()},
		{"certificate", req.HasCertificate()},
		{"skip_path_regex", req.HasSkipPathRegex()},
		{"basic_auth_enabled", req.HasBasicAuthEnabled()},
		{"basic_auth_user_attribute", req.HasBasicAuthUserAttribute()},
		{"intercept_header_auth", req.HasInterceptHeaderAuth()},
		{"cookie_domain", req.HasCookieDomain()},
		{"access_token_validity", req.HasAccessTokenValidity()},
		{"authentication_flow", req.HasAuthenticationFlow()},
	} {
		if tc.set {
			t.Errorf("%s was sent despite being unset in the spec", tc.field)
		}
	}
}

// Disabling TLS verification is an explicit false that must survive; a plain
// emptiness check would silently re-enable it.
func TestProxyBuildRequestSendsExplicitFalse(t *testing.T) {
	req := buildProxy(t, newProxyProvider(func(p *authentikv1alpha1.ProxyProvider) {
		p.Spec.InternalHostSSLValidation = ptr(false)
		p.Spec.BasicAuthEnabled = ptr(true)
		p.Spec.InterceptHeaderAuth = ptr(false)
	}))

	if !req.HasInternalHostSslValidation() || req.GetInternalHostSslValidation() {
		t.Error("internal_host_ssl_validation=false was not sent")
	}
	if !req.HasBasicAuthEnabled() || !req.GetBasicAuthEnabled() {
		t.Error("basic_auth_enabled=true was not sent")
	}
	if !req.HasInterceptHeaderAuth() || req.GetInterceptHeaderAuth() {
		t.Error("intercept_header_auth=false was not sent")
	}
}

// The basic auth trio only works as a set, so all three have to reach the
// request together.
func TestProxyBuildRequestMapsBasicAuthTrio(t *testing.T) {
	req := buildProxy(t, newProxyProvider(func(p *authentikv1alpha1.ProxyProvider) {
		p.Spec.BasicAuthEnabled = ptr(true)
		p.Spec.BasicAuthUserAttribute = "ldap_username"
		p.Spec.BasicAuthPasswordAttribute = "ldap_password"
	}))

	if !req.GetBasicAuthEnabled() {
		t.Error("basic_auth_enabled not set")
	}
	if req.GetBasicAuthUserAttribute() != "ldap_username" {
		t.Errorf("basic_auth_user_attribute = %q", req.GetBasicAuthUserAttribute())
	}
	if req.GetBasicAuthPasswordAttribute() != "ldap_password" {
		t.Errorf("basic_auth_password_attribute = %q", req.GetBasicAuthPasswordAttribute())
	}
}

// A reference that does not exist yet must surface as a not-found error, which
// is what ResultFor turns into a retry rather than a permanent failure.
func TestProxyBuildRequestReportsUnresolvableReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*authentikv1alpha1.ProxyProvider)
	}{
		{"authorization flow", func(p *authentikv1alpha1.ProxyProvider) {
			p.Spec.AuthorizationFlow = "missing-flow"
		}},
		{"authentication flow", func(p *authentikv1alpha1.ProxyProvider) {
			p.Spec.AuthenticationFlow = ptr("missing-flow")
		}},
		{"certificate", func(p *authentikv1alpha1.ProxyProvider) {
			p.Spec.Certificate = "missing-cert"
		}},
		{"property mappings", func(p *authentikv1alpha1.ProxyProvider) {
			p.Spec.PropertyMappings = []string{"missing-mapping"}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &proxyAdapter{client: newStubClient(), provider: newProxyProvider(tt.mutate)}
			_, err := a.buildRequest(context.Background())
			if err == nil {
				t.Fatal("buildRequest succeeded with an unresolvable reference")
			}
			if !authentik.IsNotFound(err) {
				t.Errorf("error %v is not a not-found, so the resource would not be retried", err)
			}
		})
	}
}

// Drift detection decides whether a DriftCorrected event is emitted. A field
// left out of the comparison means a real change is reported as a no-op.
func TestProxyEquivalent(t *testing.T) {
	base := func() *api.ProxyProvider {
		p := &api.ProxyProvider{
			Name:              "grafana",
			AuthorizationFlow: "flow-authz-uuid",
			InvalidationFlow:  "flow-invalidation-uuid",
			ExternalHost:      "https://grafana.example.com",
		}
		p.SetInternalHost("http://grafana.monitoring.svc:3000")
		p.SetMode(api.PROXYMODE_PROXY)
		p.SetBasicAuthEnabled(true)
		p.SetCookieDomain("example.com")
		return p
	}

	tests := []struct {
		name   string
		mutate func(*api.ProxyProvider)
		want   bool
	}{
		{"identical", func(*api.ProxyProvider) {}, true},
		{"name changed", func(p *api.ProxyProvider) { p.Name = "other" }, false},
		{"external host changed", func(p *api.ProxyProvider) { p.ExternalHost = "https://other" }, false},
		{"internal host changed", func(p *api.ProxyProvider) { p.SetInternalHost("http://other:80") }, false},
		{"mode changed", func(p *api.ProxyProvider) { p.SetMode(api.PROXYMODE_FORWARD_DOMAIN) }, false},
		{"basic auth toggled", func(p *api.ProxyProvider) { p.SetBasicAuthEnabled(false) }, false},
		{"cookie domain changed", func(p *api.ProxyProvider) { p.SetCookieDomain("other.com") }, false},
		// authentik computes these; treating them as drift would make every
		// reconcile emit a spurious DriftCorrected event.
		{"computed client id ignored", func(p *api.ProxyProvider) { p.ClientId = "regenerated" }, true},
		{"outpost membership ignored", func(p *api.ProxyProvider) { p.OutpostSet = []string{"edge"} }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := base()
			tt.mutate(other)
			if got := proxyEquivalent(base(), other); got != tt.want {
				t.Errorf("proxyEquivalent = %v, want %v", got, tt.want)
			}
		})
	}
}

// A nil side means the provider was never observed, which must not be reported
// as equivalent to an observed one or the first update looks like a no-op.
func TestProxyEquivalentHandlesNil(t *testing.T) {
	p := &api.ProxyProvider{Name: "grafana"}

	if proxyEquivalent(nil, p) || proxyEquivalent(p, nil) {
		t.Error("nil compared equal to an observed provider")
	}
	if !proxyEquivalent(nil, nil) {
		t.Error("two unobserved providers should compare equal")
	}
}
