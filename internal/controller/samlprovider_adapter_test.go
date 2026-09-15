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

// stubClient is a stand-in for authentik.Client that resolves references from
// in-memory maps. It is shared by the SAML and proxy adapter tests.
//
// Only the resolvers are exercised: buildRequest is pure translation, so the
// tests never need the generated HTTP client.
type stubClient struct {
	// cluster scopes managed object names; empty in most tests.
	cluster  string
	flows    map[string]string
	mappings map[string]string
	keyPairs map[string]string
}

func newStubClient() *stubClient {
	return &stubClient{
		flows: map[string]string{
			"default-authz":        "flow-authz-uuid",
			"default-invalidation": "flow-invalidation-uuid",
			"default-authn":        "flow-authn-uuid",
		},
		mappings: map[string]string{
			"email-nameid": "nameid-uuid",
			"acr":          "acr-uuid",
			"upn":          "upn-uuid",
		},
		keyPairs: map[string]string{
			"signing-cert":      "signing-cert-uuid",
			"verification-cert": "verification-cert-uuid",
			"encryption-cert":   "encryption-cert-uuid",
			"proxy-cert":        "kp-proxy-uuid",
		},
	}
}

func (c *stubClient) resolve(kind, name string, from map[string]string) (string, error) {
	if id, ok := from[name]; ok {
		return id, nil
	}
	return "", authentik.NotFound("stub resolve", kind, name)
}

func (c *stubClient) ResolveFlow(_ context.Context, slug string) (string, error) {
	return c.resolve("flow", slug, c.flows)
}

func (c *stubClient) ResolvePropertyMapping(_ context.Context, name string) (string, error) {
	return c.resolve("property mapping", name, c.mappings)
}

func (c *stubClient) ResolvePropertyMappings(ctx context.Context, names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	for _, name := range names {
		id, err := c.ResolvePropertyMapping(ctx, name)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (c *stubClient) ResolveCertificateKeyPair(_ context.Context, name string) (string, error) {
	return c.resolve("certificate key pair", name, c.keyPairs)
}

func (c *stubClient) BaseURL() string      { return "https://authentik.example.com" }
func (c *stubClient) ConnectionID() string { return "stub" }

func (c *stubClient) Cluster() string     { return c.cluster }
func (c *stubClient) InvalidateCache()    {}
func (c *stubClient) API() *api.APIClient { return nil }

func (c *stubClient) Version(context.Context) (authentik.Version, error) {
	return authentik.Version{}, nil
}

func (c *stubClient) Compatibility(context.Context) (authentik.Compatibility, error) {
	return authentik.Compatibility{}, nil
}

func (c *stubClient) ResolveProvider(context.Context, string) (int32, error) { return 0, nil }

func (c *stubClient) ResolveServiceConnection(context.Context, string) (string, error) {
	return "", nil
}

// ptr is a terse literal-to-pointer helper for optional spec fields.
func ptr[T any](v T) *T { return &v }

// newSAMLProvider returns a minimally valid provider the tests can amend.
func newSAMLProvider(mutate func(*authentikv1alpha1.SAMLProvider)) *authentikv1alpha1.SAMLProvider {
	p := &authentikv1alpha1.SAMLProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "grafana", Namespace: "default"},
		Spec: authentikv1alpha1.SAMLProviderSpec{
			ProviderCommonSpec: authentikv1alpha1.ProviderCommonSpec{
				AuthorizationFlow: authentikv1alpha1.FlowReference{Name: "default-authz"},
				InvalidationFlow:  authentikv1alpha1.FlowReference{Name: "default-invalidation"},
			},
			ACSURL: "https://grafana.example.com/saml/acs",
		},
	}
	if mutate != nil {
		mutate(p)
	}
	return p
}

func buildSAML(t *testing.T, p *authentikv1alpha1.SAMLProvider) *api.SAMLProviderRequest {
	t.Helper()
	a := &samlAdapter{client: newStubClient(), kube: referenceFixture(t, "default", "default-authz", "default-invalidation", "default-authn", "openid", "email", "profile", "claims", "upn", "nameid", "authn-context", "saml-mapping", "proxy-cert", "signing-cert", "verification-cert", "encryption-cert", "docker-ca", "docker-cert", "absent-ok", "acr", "email-nameid"), provider: p}
	req, err := a.buildRequest(context.Background())
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	return req
}

// The three required positional arguments are what authentik rejects the whole
// request over, so they must come from the spec and not from the resource name
// by accident.
func TestSAMLBuildRequestSetsRequiredFields(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(nil))

	if req.Name != "grafana" {
		t.Errorf("Name = %q, want grafana", req.Name)
	}
	if req.AuthorizationFlow != "default-authz-uuid" {
		t.Errorf("AuthorizationFlow = %q, want the resolved UUID", req.AuthorizationFlow)
	}
	if req.InvalidationFlow != "default-invalidation-uuid" {
		t.Errorf("InvalidationFlow = %q, want the resolved UUID", req.InvalidationFlow)
	}
	if req.AcsUrl != "https://grafana.example.com/saml/acs" {
		t.Errorf("AcsUrl = %q", req.AcsUrl)
	}
}

// spec.name overrides the resource name, and it is the name adoption collides
// on, so getting it wrong silently creates a second provider.
func TestSAMLBuildRequestPrefersSpecName(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(func(p *authentikv1alpha1.SAMLProvider) {
		p.Spec.Name = "grafana-prod"
	}))

	if req.Name != "grafana-prod" {
		t.Errorf("Name = %q, want the spec override", req.Name)
	}
}

// Certificate key pairs are given by name and must reach authentik as UUIDs;
// sending the name through unchanged is accepted by the schema and then fails
// deep inside authentik.
func TestSAMLBuildRequestResolvesKeyPairsToUUIDs(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(func(p *authentikv1alpha1.SAMLProvider) {
		p.Spec.SigningKeyPair = &authentikv1alpha1.CertificateKeyPairReference{Name: "signing-cert"}
		p.Spec.VerificationKeyPair = &authentikv1alpha1.CertificateKeyPairReference{Name: "verification-cert"}
		p.Spec.EncryptionKeyPair = &authentikv1alpha1.CertificateKeyPairReference{Name: "encryption-cert"}
	}))

	for _, tc := range []struct {
		field string
		got   string
		want  string
	}{
		{"signing_kp", req.GetSigningKp(), "signing-cert-uuid"},
		{"verification_kp", req.GetVerificationKp(), "verification-cert-uuid"},
		{"encryption_kp", req.GetEncryptionKp(), "encryption-cert-uuid"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %q, want %q", tc.field, tc.got, tc.want)
		}
	}
}

// The NameID and AuthnContextClassRef mappings are single property mappings,
// resolved through a different call than the propertyMappings list; mixing the
// two up silently drops one of them.
func TestSAMLBuildRequestResolvesMappings(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(func(p *authentikv1alpha1.SAMLProvider) {
		p.Spec.PropertyMappings = []authentikv1alpha1.PropertyMappingReference{{Name: "upn"}, {Name: "acr"}}
		p.Spec.NameIDMapping = &authentikv1alpha1.PropertyMappingReference{Name: "email-nameid"}
		p.Spec.AuthnContextClassRefMapping = &authentikv1alpha1.PropertyMappingReference{Name: "acr"}
	}))

	want := []string{"upn-uuid", "acr-uuid"}
	got := req.GetPropertyMappings()
	if len(got) != len(want) {
		t.Fatalf("property_mappings = %v, want %v", got, want)
	}
	for i := range want {
		// Order is meaningful: property mappings are applied in sequence.
		if got[i] != want[i] {
			t.Errorf("property_mappings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if req.GetNameIdMapping() != "email-nameid-uuid" {
		t.Errorf("name_id_mapping = %q", req.GetNameIdMapping())
	}
	if req.GetAuthnContextClassRefMapping() != "acr-uuid" {
		t.Errorf("authn_context_class_ref_mapping = %q", req.GetAuthnContextClassRefMapping())
	}
}

// Enum values are passed through verbatim. authentik keys them by the full W3C
// or OASIS URI, so any rewriting here produces a request it rejects.
func TestSAMLBuildRequestPassesEnumsThroughVerbatim(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(func(p *authentikv1alpha1.SAMLProvider) {
		p.Spec.DigestAlgorithm = "http://www.w3.org/2001/04/xmlenc#sha256"
		p.Spec.SignatureAlgorithm = "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256"
		p.Spec.SPBinding = "post"
		p.Spec.SLSBinding = "redirect"
		p.Spec.LogoutMethod = "backchannel"
		p.Spec.DefaultNameIDPolicy = "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent"
	}))

	if got := string(req.GetDigestAlgorithm()); got != "http://www.w3.org/2001/04/xmlenc#sha256" {
		t.Errorf("digest_algorithm = %q", got)
	}
	if got := string(req.GetSignatureAlgorithm()); got != "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256" {
		t.Errorf("signature_algorithm = %q", got)
	}
	if got := string(req.GetSpBinding()); got != "post" {
		t.Errorf("sp_binding = %q", got)
	}
	if got := string(req.GetSlsBinding()); got != "redirect" {
		t.Errorf("sls_binding = %q", got)
	}
	if got := string(req.GetLogoutMethod()); got != "backchannel" {
		t.Errorf("logout_method = %q", got)
	}
	if got := string(req.GetDefaultNameIdPolicy()); got != "urn:oasis:names:tc:SAML:2.0:nameid-format:persistent" {
		t.Errorf("default_name_id_policy = %q", got)
	}
}

// An unset optional field must be absent from the request, not present as a
// zero value: sending sign_assertion=false would overwrite whatever authentik
// or an administrator had configured.
func TestSAMLBuildRequestOmitsUnsetOptionalFields(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(nil))

	for _, tc := range []struct {
		field string
		set   bool
	}{
		{"sls_url", req.HasSlsUrl()},
		{"audience", req.HasAudience()},
		{"issuer_override", req.HasIssuerOverride()},
		{"assertion_valid_not_before", req.HasAssertionValidNotBefore()},
		{"session_valid_not_on_or_after", req.HasSessionValidNotOnOrAfter()},
		{"sign_assertion", req.HasSignAssertion()},
		{"sign_response", req.HasSignResponse()},
		{"sign_logout_request", req.HasSignLogoutRequest()},
		{"sign_logout_response", req.HasSignLogoutResponse()},
		{"digest_algorithm", req.HasDigestAlgorithm()},
		{"signing_kp", req.HasSigningKp()},
		{"authentication_flow", req.HasAuthenticationFlow()},
	} {
		if tc.set {
			t.Errorf("%s was sent despite being unset in the spec", tc.field)
		}
	}
}

// An explicit false is a real instruction and must survive, which a plain
// emptiness check would swallow.
func TestSAMLBuildRequestSendsExplicitFalse(t *testing.T) {
	req := buildSAML(t, newSAMLProvider(func(p *authentikv1alpha1.SAMLProvider) {
		p.Spec.SignAssertion = ptr(false)
		p.Spec.SignResponse = ptr(true)
	}))

	if !req.HasSignAssertion() || req.GetSignAssertion() {
		t.Error("sign_assertion=false was not sent")
	}
	if !req.HasSignResponse() || !req.GetSignResponse() {
		t.Error("sign_response=true was not sent")
	}
}

// A reference that does not exist yet must surface as a not-found error, since
// that is what ResultFor turns into a retry rather than a permanent failure.
func TestSAMLBuildRequestReportsUnresolvableReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*authentikv1alpha1.SAMLProvider)
	}{
		{"authorization flow", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.AuthorizationFlow = authentikv1alpha1.FlowReference{Name: "missing-flow"}
		}},
		{"invalidation flow", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.InvalidationFlow = authentikv1alpha1.FlowReference{Name: "missing-flow"}
		}},
		{"authentication flow", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.AuthenticationFlow = &authentikv1alpha1.FlowReference{Name: "missing-flow"}
		}},
		{"signing key pair", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.SigningKeyPair = &authentikv1alpha1.CertificateKeyPairReference{Name: "missing-cert"}
		}},
		{"nameID mapping", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.NameIDMapping = &authentikv1alpha1.PropertyMappingReference{Name: "missing-mapping"}
		}},
		{"property mappings", func(p *authentikv1alpha1.SAMLProvider) {
			p.Spec.PropertyMappings = []authentikv1alpha1.PropertyMappingReference{{Name: "upn"}, {Name: "missing-mapping"}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &samlAdapter{client: newStubClient(), kube: referenceFixture(t, "default", "default-authz", "default-invalidation", "default-authn", "openid", "email", "profile", "claims", "upn", "nameid", "authn-context", "saml-mapping", "proxy-cert", "signing-cert", "verification-cert", "encryption-cert", "docker-ca", "docker-cert", "absent-ok", "acr", "email-nameid"), provider: newSAMLProvider(tt.mutate)}
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
func TestSAMLEquivalent(t *testing.T) {
	base := func() *api.SAMLProvider {
		p := &api.SAMLProvider{
			Name:              "grafana",
			AuthorizationFlow: "flow-authz-uuid",
			InvalidationFlow:  "flow-invalidation-uuid",
			AcsUrl:            "https://grafana.example.com/saml/acs",
		}
		p.SetAudience("grafana")
		p.SetSignAssertion(true)
		p.SetSigningKp("signing-cert-uuid")
		p.SetSpBinding(api.SAMLBINDINGSENUM_POST)
		return p
	}

	tests := []struct {
		name   string
		mutate func(*api.SAMLProvider)
		want   bool
	}{
		{"identical", func(*api.SAMLProvider) {}, true},
		{"name changed", func(p *api.SAMLProvider) { p.Name = "other" }, false},
		{"acs url changed", func(p *api.SAMLProvider) { p.AcsUrl = "https://other/acs" }, false},
		{"audience changed", func(p *api.SAMLProvider) { p.SetAudience("other") }, false},
		{"signing key changed", func(p *api.SAMLProvider) { p.SetSigningKp("kp-other") }, false},
		{"sign assertion toggled", func(p *api.SAMLProvider) { p.SetSignAssertion(false) }, false},
		{"binding changed", func(p *api.SAMLProvider) { p.SetSpBinding(api.SAMLBINDINGSENUM_REDIRECT) }, false},
		// authentik computes these; treating them as drift would make every
		// reconcile emit a spurious DriftCorrected event.
		{"computed metadata url ignored", func(p *api.SAMLProvider) { p.UrlDownloadMetadata = "https://x" }, true},
		{"computed issuer ignored", func(p *api.SAMLProvider) { p.UrlIssuer = "https://y" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			other := base()
			tt.mutate(other)
			if got := samlEquivalent(base(), other); got != tt.want {
				t.Errorf("samlEquivalent = %v, want %v", got, tt.want)
			}
		})
	}
}

// A nil side means the provider was never observed, which must not be reported
// as equivalent to an observed one or the first update looks like a no-op.
func TestSAMLEquivalentHandlesNil(t *testing.T) {
	p := &api.SAMLProvider{Name: "grafana"}

	if samlEquivalent(nil, p) || samlEquivalent(p, nil) {
		t.Error("nil compared equal to an observed provider")
	}
	if !samlEquivalent(nil, nil) {
		t.Error("two unobserved providers should compare equal")
	}
}
