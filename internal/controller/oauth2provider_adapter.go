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
	"sigs.k8s.io/controller-runtime/pkg/client"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// oauth2Adapter implements RemoteAdapter for an OAuth2 provider.
type oauth2Adapter struct {
	client authentik.Client
	// kube reads the Flow, PropertyMapping and CertificateKeyPair
	// resources this provider references.
	kube     client.Client
	provider *authentikv1alpha1.OAuth2Provider

	// clientSecret is the secret to send, either taken from a referenced
	// Secret or left empty so authentik generates one.
	clientSecret string

	// observed holds the provider as last read from authentik, so the
	// controller can surface the generated client id without re-fetching.
	observed *api.OAuth2Provider
}

func (a *oauth2Adapter) Kind() string { return "OAuth2 provider" }

func (a *oauth2Adapter) DesiredName() string { return a.provider.ProviderName() }

func (a *oauth2Adapter) Exists(ctx context.Context, id string) (bool, error) {
	pk, err := parseProviderID(id)
	if err != nil {
		// A malformed recorded ID is treated as absent rather than fatal, so
		// the resource recovers by looking the provider up by name.
		return false, nil
	}

	found, resp, err := a.client.API().ProvidersAPI.ProvidersOauth2Retrieve(ctx, pk).Execute()
	if err != nil {
		if mapped := authentik.MapResponseError("retrieve oauth2 provider", resp, err); authentik.IsNotFound(mapped) {
			return false, nil
		}
		return false, authentik.MapResponseError("retrieve oauth2 provider", resp, err)
	}
	a.observed = found
	return true, nil
}

func (a *oauth2Adapter) FindByName(ctx context.Context) (string, error) {
	const op = "list oauth2 providers"
	name := a.DesiredName()

	list, resp, err := a.client.API().ProvidersAPI.ProvidersOauth2List(ctx).
		Name(name).PageSize(100).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}

	// The name filter is not guaranteed to be exact, so narrow client-side.
	var matches []api.OAuth2Provider
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "OAuth2 provider", name)
	case 1:
		a.observed = &matches[0]
		return strconv.FormatInt(int64(matches[0].Pk), 10), nil
	default:
		return "", authentik.Ambiguous(op, "OAuth2 provider", name, len(matches))
	}
}

func (a *oauth2Adapter) Create(ctx context.Context) (string, error) {
	const op = "create oauth2 provider"

	req, err := a.buildRequest(ctx)
	if err != nil {
		return "", err
	}

	created, resp, err := a.client.API().ProvidersAPI.ProvidersOauth2Create(ctx).
		OAuth2ProviderRequest(*req).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return strconv.FormatInt(int64(created.Pk), 10), nil
}

func (a *oauth2Adapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update oauth2 provider"

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
	updated, resp, err := a.client.API().ProvidersAPI.ProvidersOauth2Update(ctx, pk).
		OAuth2ProviderRequest(*req).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !oauth2Equivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *oauth2Adapter) Delete(ctx context.Context, id string) error {
	const op = "delete oauth2 provider"

	pk, err := parseProviderID(id)
	if err != nil {
		// Nothing sensible to delete; treat as already gone.
		return nil
	}

	resp, err := a.client.API().ProvidersAPI.ProvidersOauth2Destroy(ctx, pk).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec into an authentik request, resolving every
// human-readable reference to the UUID authentik expects.
func (a *oauth2Adapter) buildRequest(ctx context.Context) (*api.OAuth2ProviderRequest, error) {
	spec := a.provider.Spec

	authzFlow, err := resolveFlowRef(ctx, a.kube, a.provider.Namespace, "spec.authorizationFlow", spec.AuthorizationFlow)
	if err != nil {
		return nil, err
	}
	invalidationFlow, err := resolveFlowRef(ctx, a.kube, a.provider.Namespace, "spec.invalidationFlow", spec.InvalidationFlow)
	if err != nil {
		return nil, err
	}

	req := api.NewOAuth2ProviderRequest(a.DesiredName(), authzFlow, invalidationFlow, a.redirectURIs())

	authnFlow, err := resolveOptionalFlowRef(ctx, a.kube, a.provider.Namespace, "spec.authenticationFlow", spec.AuthenticationFlow)
	if err != nil {
		return nil, err
	}
	if authnFlow != "" {
		req.SetAuthenticationFlow(authnFlow)
	}

	mappings, err := resolvePropertyMappingRefs(ctx, a.kube, a.provider.Namespace, "spec.propertyMappings", spec.PropertyMappings)
	if err != nil {
		return nil, err
	}
	if len(mappings) > 0 {
		req.SetPropertyMappings(mappings)
	}

	signingKey, err := resolveKeyPairRef(ctx, a.kube, a.provider.Namespace, "spec.signingKeyPair", spec.SigningKeyPair)
	if err != nil {
		return nil, err
	}
	if signingKey != "" {
		req.SetSigningKey(signingKey)
	}
	encryptionKey, err := resolveKeyPairRef(ctx, a.kube, a.provider.Namespace, "spec.encryptionKeyPair", spec.EncryptionKeyPair)
	if err != nil {
		return nil, err
	}
	if encryptionKey != "" {
		req.SetEncryptionKey(encryptionKey)
	}

	if spec.ClientType != "" {
		req.SetClientType(api.ClientTypeEnum(spec.ClientType))
	}
	if len(spec.GrantTypes) > 0 {
		grants := make([]api.GrantTypeEnum, 0, len(spec.GrantTypes))
		for _, g := range spec.GrantTypes {
			grants = append(grants, api.GrantTypeEnum(g))
		}
		req.SetGrantTypes(grants)
	}
	if spec.ClientID != "" {
		req.SetClientId(spec.ClientID)
	}
	if a.clientSecret != "" {
		req.SetClientSecret(a.clientSecret)
	}
	setIfNotEmpty(spec.AccessCodeValidity, req.SetAccessCodeValidity)
	setIfNotEmpty(spec.AccessTokenValidity, req.SetAccessTokenValidity)
	setIfNotEmpty(spec.RefreshTokenValidity, req.SetRefreshTokenValidity)
	setIfNotEmpty(spec.SubMode, func(v string) { req.SetSubMode(api.SubModeEnum(v)) })
	setIfNotEmpty(spec.IssuerMode, func(v string) { req.SetIssuerMode(api.IssuerModeEnum(v)) })
	setIfNotEmpty(spec.LogoutURI, req.SetLogoutUri)
	setIfNotEmpty(spec.LogoutMethod, func(v string) {
		req.SetLogoutMethod(api.OAuth2ProviderLogoutMethodEnum(v))
	})
	if spec.IncludeClaimsInIDToken != nil {
		req.SetIncludeClaimsInIdToken(*spec.IncludeClaimsInIDToken)
	}

	return req, nil
}

func (a *oauth2Adapter) redirectURIs() []api.RedirectURIRequest {
	uris := make([]api.RedirectURIRequest, 0, len(a.provider.Spec.RedirectURIs))
	for _, u := range a.provider.Spec.RedirectURIs {
		mode := u.MatchingMode
		if mode == "" {
			mode = "strict"
		}
		uris = append(uris, api.RedirectURIRequest{
			MatchingMode: api.MatchingModeEnum(mode),
			Url:          u.URL,
		})
	}
	return uris
}

// oauth2Equivalent reports whether two provider states are the same in the
// fields this operator manages. Fields authentik computes are ignored, so a
// server-side default never looks like drift.
func oauth2Equivalent(a, b *api.OAuth2Provider) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.AuthorizationFlow == b.AuthorizationFlow &&
		a.InvalidationFlow == b.InvalidationFlow &&
		a.GetClientId() == b.GetClientId() &&
		a.GetClientType() == b.GetClientType() &&
		a.GetSubMode() == b.GetSubMode() &&
		a.GetIssuerMode() == b.GetIssuerMode() &&
		a.GetIncludeClaimsInIdToken() == b.GetIncludeClaimsInIdToken()
}

// setIfNotEmpty calls set only for a non-empty value, keeping buildRequest
// free of a dozen near-identical if statements.
func setIfNotEmpty(value string, set func(string)) {
	if value != "" {
		set(value)
	}
}

// parseProviderID converts a recorded remote ID into authentik's numeric
// primary key.
func parseProviderID(id string) (int32, error) {
	pk, err := strconv.ParseInt(id, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(pk), nil
}

// setupURLs fetches the provider's OIDC discovery endpoints.
//
// It is deliberately best effort and returns nil on any failure: these URLs
// are a convenience published next to the credentials, and losing them must
// never stop the credentials themselves from reaching the workload.
func (a *oauth2Adapter) setupURLs(ctx context.Context, providerID *int32) *api.OAuth2ProviderSetupURLs {
	if providerID == nil {
		return nil
	}
	urls, _, err := a.client.API().ProvidersAPI.
		ProvidersOauth2SetupUrlsRetrieve(ctx, *providerID).Execute()
	if err != nil {
		return nil
	}
	return urls
}
