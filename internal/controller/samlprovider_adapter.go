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
	"strconv"

	api "goauthentik.io/api/v3"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// samlAdapter implements RemoteAdapter for a SAML provider.
type samlAdapter struct {
	client   authentik.Client
	provider *authentikv1alpha1.SAMLProvider

	// observed holds the provider as last read from authentik, so the
	// controller can surface the metadata and issuer URLs without re-fetching.
	observed *api.SAMLProvider
}

func (a *samlAdapter) Kind() string { return "SAML provider" }

func (a *samlAdapter) DesiredName() string { return a.provider.ProviderName() }

func (a *samlAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve saml provider"

	pk, err := parseProviderID(id)
	if err != nil {
		// A malformed recorded ID is treated as absent rather than fatal, so
		// the resource recovers by looking the provider up by name.
		return false, nil
	}

	found, resp, err := a.client.API().ProvidersAPI.ProvidersSamlRetrieve(ctx, pk).Execute()
	if err != nil {
		mapped := authentik.MapResponseError(op, resp, err)
		if authentik.IsNotFound(mapped) {
			return false, nil
		}
		return false, mapped
	}
	a.observed = found
	return true, nil
}

func (a *samlAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list saml providers"
	name := a.DesiredName()

	list, resp, err := a.client.API().ProvidersAPI.ProvidersSamlList(ctx).
		Name(name).PageSize(100).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}

	// The name filter is not guaranteed to be exact, so narrow client-side.
	var matches []api.SAMLProvider
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "SAML provider", name)
	case 1:
		a.observed = &matches[0]
		return strconv.FormatInt(int64(matches[0].Pk), 10), nil
	default:
		return "", authentik.Ambiguous(op, "SAML provider", name, len(matches))
	}
}

func (a *samlAdapter) Create(ctx context.Context) (string, error) {
	const op = "create saml provider"

	req, err := a.buildRequest(ctx)
	if err != nil {
		return "", err
	}

	created, resp, err := a.client.API().ProvidersAPI.ProvidersSamlCreate(ctx).
		SAMLProviderRequest(*req).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return strconv.FormatInt(int64(created.Pk), 10), nil
}

func (a *samlAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update saml provider"

	pk, err := parseProviderID(id)
	if err != nil {
		return false, err
	}

	req, err := a.buildRequest(ctx)
	if err != nil {
		return false, err
	}

	// A full update is used rather than a partial one because the request is
	// built from the complete spec every time; sending it whole keeps the
	// remote object converging on the declared state.
	updated, resp, err := a.client.API().ProvidersAPI.ProvidersSamlUpdate(ctx, pk).
		SAMLProviderRequest(*req).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !samlEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *samlAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete saml provider"

	pk, err := parseProviderID(id)
	if err != nil {
		// Nothing sensible to delete; treat as already gone.
		return nil
	}

	resp, err := a.client.API().ProvidersAPI.ProvidersSamlDestroy(ctx, pk).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec into an authentik request, resolving every
// human-readable reference to the UUID authentik expects.
func (a *samlAdapter) buildRequest(ctx context.Context) (*api.SAMLProviderRequest, error) {
	spec := a.provider.Spec

	authzFlow, err := a.client.ResolveFlow(ctx, spec.AuthorizationFlow)
	if err != nil {
		return nil, err
	}
	invalidationFlow, err := a.client.ResolveFlow(ctx, spec.InvalidationFlow)
	if err != nil {
		return nil, err
	}

	req := api.NewSAMLProviderRequest(a.DesiredName(), authzFlow, invalidationFlow, spec.ACSURL)

	if spec.AuthenticationFlow != nil && *spec.AuthenticationFlow != "" {
		authnFlow, err := a.client.ResolveFlow(ctx, *spec.AuthenticationFlow)
		if err != nil {
			return nil, err
		}
		req.SetAuthenticationFlow(authnFlow)
	}

	if len(spec.PropertyMappings) > 0 {
		mappings, err := a.client.ResolvePropertyMappings(ctx, spec.PropertyMappings)
		if err != nil {
			return nil, err
		}
		req.SetPropertyMappings(mappings)
	}

	// The NameID and AuthnContextClassRef mappings are single property
	// mappings rather than a list, so they resolve one at a time.
	if spec.NameIDMapping != "" {
		mapping, err := a.client.ResolvePropertyMapping(ctx, spec.NameIDMapping)
		if err != nil {
			return nil, err
		}
		req.SetNameIdMapping(mapping)
	}
	if spec.AuthnContextClassRefMapping != "" {
		mapping, err := a.client.ResolvePropertyMapping(ctx, spec.AuthnContextClassRefMapping)
		if err != nil {
			return nil, err
		}
		req.SetAuthnContextClassRefMapping(mapping)
	}

	if err := a.setKeyPairs(ctx, req); err != nil {
		return nil, err
	}

	setIfNotEmpty(spec.SLSURL, req.SetSlsUrl)
	setIfNotEmpty(spec.Audience, req.SetAudience)
	setIfNotEmpty(spec.IssuerOverride, req.SetIssuerOverride)
	setIfNotEmpty(spec.AssertionValidNotBefore, req.SetAssertionValidNotBefore)
	setIfNotEmpty(spec.AssertionValidNotOnOrAfter, req.SetAssertionValidNotOnOrAfter)
	setIfNotEmpty(spec.SessionValidNotOnOrAfter, req.SetSessionValidNotOnOrAfter)
	setIfNotEmpty(spec.DefaultRelayState, req.SetDefaultRelayState)

	setIfNotEmpty(string(spec.DigestAlgorithm), func(v string) {
		req.SetDigestAlgorithm(api.DigestAlgorithmEnum(v))
	})
	setIfNotEmpty(string(spec.SignatureAlgorithm), func(v string) {
		req.SetSignatureAlgorithm(api.SignatureAlgorithmEnum(v))
	})
	setIfNotEmpty(string(spec.SPBinding), func(v string) {
		req.SetSpBinding(api.SAMLBindingsEnum(v))
	})
	setIfNotEmpty(string(spec.SLSBinding), func(v string) {
		req.SetSlsBinding(api.SAMLBindingsEnum(v))
	})
	setIfNotEmpty(string(spec.LogoutMethod), func(v string) {
		req.SetLogoutMethod(api.SAMLLogoutMethods(v))
	})
	setIfNotEmpty(string(spec.DefaultNameIDPolicy), func(v string) {
		req.SetDefaultNameIdPolicy(api.SAMLNameIDPolicyEnum(v))
	})

	setIfNotNil(spec.SignAssertion, req.SetSignAssertion)
	setIfNotNil(spec.SignResponse, req.SetSignResponse)
	setIfNotNil(spec.SignLogoutRequest, req.SetSignLogoutRequest)
	setIfNotNil(spec.SignLogoutResponse, req.SetSignLogoutResponse)

	return req, nil
}

// setKeyPairs resolves the three certificate key pair names onto the request.
func (a *samlAdapter) setKeyPairs(ctx context.Context, req *api.SAMLProviderRequest) error {
	spec := a.provider.Spec

	for _, kp := range []struct {
		name string
		set  func(string)
	}{
		{spec.SigningKeyPair, req.SetSigningKp},
		{spec.VerificationKeyPair, req.SetVerificationKp},
		{spec.EncryptionKeyPair, req.SetEncryptionKp},
	} {
		if kp.name == "" {
			continue
		}
		resolved, err := a.client.ResolveCertificateKeyPair(ctx, kp.name)
		if err != nil {
			return err
		}
		kp.set(resolved)
	}
	return nil
}

// samlManagedState is the subset of a SAML provider this operator owns,
// flattened into a comparable value.
//
// Drift is then one equality check rather than a chain of two dozen
// comparisons, which is both easier to keep complete and easier to read. Every
// field authentik computes is deliberately absent, so a server-side default
// never looks like drift.
type samlManagedState struct {
	name              string
	authorizationFlow string
	invalidationFlow  string

	acsURL         string
	slsURL         string
	audience       string
	issuerOverride string

	assertionValidNotBefore    string
	assertionValidNotOnOrAfter string
	sessionValidNotOnOrAfter   string

	nameIDMapping               string
	authnContextClassRefMapping string

	digestAlgorithm    api.DigestAlgorithmEnum
	signatureAlgorithm api.SignatureAlgorithmEnum

	signingKp      string
	verificationKp string
	encryptionKp   string

	signAssertion      bool
	signResponse       bool
	signLogoutRequest  bool
	signLogoutResponse bool

	spBinding    api.SAMLBindingsEnum
	slsBinding   api.SAMLBindingsEnum
	logoutMethod api.SAMLLogoutMethods

	defaultRelayState   string
	defaultNameIDPolicy api.SAMLNameIDPolicyEnum
}

// samlStateOf projects a provider onto the fields this operator manages.
func samlStateOf(p *api.SAMLProvider) samlManagedState {
	return samlManagedState{
		name:                        p.Name,
		authorizationFlow:           p.AuthorizationFlow,
		invalidationFlow:            p.InvalidationFlow,
		acsURL:                      p.AcsUrl,
		slsURL:                      p.GetSlsUrl(),
		audience:                    p.GetAudience(),
		issuerOverride:              p.GetIssuerOverride(),
		assertionValidNotBefore:     p.GetAssertionValidNotBefore(),
		assertionValidNotOnOrAfter:  p.GetAssertionValidNotOnOrAfter(),
		sessionValidNotOnOrAfter:    p.GetSessionValidNotOnOrAfter(),
		nameIDMapping:               p.GetNameIdMapping(),
		authnContextClassRefMapping: p.GetAuthnContextClassRefMapping(),
		digestAlgorithm:             p.GetDigestAlgorithm(),
		signatureAlgorithm:          p.GetSignatureAlgorithm(),
		signingKp:                   p.GetSigningKp(),
		verificationKp:              p.GetVerificationKp(),
		encryptionKp:                p.GetEncryptionKp(),
		signAssertion:               p.GetSignAssertion(),
		signResponse:                p.GetSignResponse(),
		signLogoutRequest:           p.GetSignLogoutRequest(),
		signLogoutResponse:          p.GetSignLogoutResponse(),
		spBinding:                   p.GetSpBinding(),
		slsBinding:                  p.GetSlsBinding(),
		logoutMethod:                p.GetLogoutMethod(),
		defaultRelayState:           p.GetDefaultRelayState(),
		defaultNameIDPolicy:         p.GetDefaultNameIdPolicy(),
	}
}

// samlEquivalent reports whether two provider states are the same in the
// fields this operator manages.
func samlEquivalent(a, b *api.SAMLProvider) bool {
	if a == nil || b == nil {
		return a == b
	}
	return samlStateOf(a) == samlStateOf(b)
}

// setIfNotNil calls set only for a value the spec actually carries.
//
// A nil pointer means "not specified", which must leave authentik's own
// default in place rather than sending the zero value.
func setIfNotNil[T any](value *T, set func(T)) {
	if value != nil {
		set(*value)
	}
}
