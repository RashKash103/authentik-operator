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

// proxyAdapter implements RemoteAdapter for a proxy provider.
type proxyAdapter struct {
	client authentik.Client
	// kube reads the Flow, PropertyMapping and CertificateKeyPair
	// resources this provider references.
	kube     client.Client
	provider *authentikv1alpha1.ProxyProvider

	// observed holds the provider as last read from authentik, so the
	// controller can surface the serving outposts without re-fetching.
	observed *api.ProxyProvider
}

func (a *proxyAdapter) Kind() string { return "proxy provider" }

func (a *proxyAdapter) DesiredName() string { return a.provider.ProviderName() }

func (a *proxyAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve proxy provider"

	pk, err := parseProviderID(id)
	if err != nil {
		// A malformed recorded ID is treated as absent rather than fatal, so
		// the resource recovers by looking the provider up by name.
		return false, nil
	}

	found, resp, err := a.client.API().ProvidersAPI.ProvidersProxyRetrieve(ctx, pk).Execute()
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

func (a *proxyAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list proxy providers"
	name := a.DesiredName()

	// The proxy endpoint offers no exact name filter, only the
	// case-insensitive name_iexact, so the case-sensitive comparison that
	// decides adoption has to happen client-side below.
	list, resp, err := a.client.API().ProvidersAPI.ProvidersProxyList(ctx).
		NameIexact(name).PageSize(100).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}

	var matches []api.ProxyProvider
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "proxy provider", name)
	case 1:
		a.observed = &matches[0]
		return strconv.FormatInt(int64(matches[0].Pk), 10), nil
	default:
		return "", authentik.Ambiguous(op, "proxy provider", name, len(matches))
	}
}

func (a *proxyAdapter) Create(ctx context.Context) (string, error) {
	const op = "create proxy provider"

	req, err := a.buildRequest(ctx)
	if err != nil {
		return "", err
	}

	created, resp, err := a.client.API().ProvidersAPI.ProvidersProxyCreate(ctx).
		ProxyProviderRequest(*req).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return strconv.FormatInt(int64(created.Pk), 10), nil
}

func (a *proxyAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update proxy provider"

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
	updated, resp, err := a.client.API().ProvidersAPI.ProvidersProxyUpdate(ctx, pk).
		ProxyProviderRequest(*req).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !proxyEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *proxyAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete proxy provider"

	pk, err := parseProviderID(id)
	if err != nil {
		// Nothing sensible to delete; treat as already gone.
		return nil
	}

	resp, err := a.client.API().ProvidersAPI.ProvidersProxyDestroy(ctx, pk).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec into an authentik request, resolving every
// human-readable reference to the UUID authentik expects.
func (a *proxyAdapter) buildRequest(ctx context.Context) (*api.ProxyProviderRequest, error) {
	spec := a.provider.Spec

	authzFlow, err := resolveFlowRef(ctx, a.kube, a.provider.Namespace, "spec.authorizationFlow", spec.AuthorizationFlow)
	if err != nil {
		return nil, err
	}
	invalidationFlow, err := resolveFlowRef(ctx, a.kube, a.provider.Namespace, "spec.invalidationFlow", spec.InvalidationFlow)
	if err != nil {
		return nil, err
	}

	req := api.NewProxyProviderRequest(a.DesiredName(), authzFlow, invalidationFlow, spec.ExternalHost)

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

	certificate, err := resolveKeyPairRef(ctx, a.kube, a.provider.Namespace, "spec.certificate", spec.Certificate)
	if err != nil {
		return nil, err
	}
	if certificate != "" {
		req.SetCertificate(certificate)
	}

	// Only proxy mode has an upstream. The CRD already refuses the
	// combination, so anything reaching here with a forwarding mode came from
	// a cluster whose schema predates that rule; dropping the field keeps the
	// request consistent rather than letting authentik silently ignore it.
	if spec.InternalHost != "" && spec.Mode != authentikv1alpha1.ProxyModeForwardSingle &&
		spec.Mode != authentikv1alpha1.ProxyModeForwardDomain {
		req.SetInternalHost(spec.InternalHost)
	}

	setIfNotEmpty(string(spec.Mode), func(v string) { req.SetMode(api.ProxyMode(v)) })
	setIfNotEmpty(spec.SkipPathRegex, req.SetSkipPathRegex)
	setIfNotEmpty(spec.BasicAuthUserAttribute, req.SetBasicAuthUserAttribute)
	setIfNotEmpty(spec.BasicAuthPasswordAttribute, req.SetBasicAuthPasswordAttribute)
	setIfNotEmpty(spec.CookieDomain, req.SetCookieDomain)
	setIfNotEmpty(spec.AccessTokenValidity, req.SetAccessTokenValidity)
	setIfNotEmpty(spec.RefreshTokenValidity, req.SetRefreshTokenValidity)

	setIfNotNil(spec.InternalHostSSLValidation, req.SetInternalHostSslValidation)
	setIfNotNil(spec.BasicAuthEnabled, req.SetBasicAuthEnabled)
	setIfNotNil(spec.InterceptHeaderAuth, req.SetInterceptHeaderAuth)

	return req, nil
}

// proxyManagedState is the subset of a proxy provider this operator owns,
// flattened into a comparable value so drift is one equality check. Every
// field authentik computes — the client id and the outpost membership — is
// deliberately absent, so a server-side change never looks like drift.
type proxyManagedState struct {
	name              string
	authorizationFlow string
	invalidationFlow  string

	externalHost              string
	internalHost              string
	internalHostSSLValidation bool
	certificate               string
	skipPathRegex             string

	basicAuthEnabled           bool
	basicAuthUserAttribute     string
	basicAuthPasswordAttribute string

	mode                api.ProxyMode
	interceptHeaderAuth bool
	cookieDomain        string

	accessTokenValidity  string
	refreshTokenValidity string
}

// proxyStateOf projects a provider onto the fields this operator manages.
func proxyStateOf(p *api.ProxyProvider) proxyManagedState {
	return proxyManagedState{
		name:                       p.Name,
		authorizationFlow:          p.AuthorizationFlow,
		invalidationFlow:           p.InvalidationFlow,
		externalHost:               p.ExternalHost,
		internalHost:               p.GetInternalHost(),
		internalHostSSLValidation:  p.GetInternalHostSslValidation(),
		certificate:                p.GetCertificate(),
		skipPathRegex:              p.GetSkipPathRegex(),
		basicAuthEnabled:           p.GetBasicAuthEnabled(),
		basicAuthUserAttribute:     p.GetBasicAuthUserAttribute(),
		basicAuthPasswordAttribute: p.GetBasicAuthPasswordAttribute(),
		mode:                       p.GetMode(),
		interceptHeaderAuth:        p.GetInterceptHeaderAuth(),
		cookieDomain:               p.GetCookieDomain(),
		accessTokenValidity:        p.GetAccessTokenValidity(),
		refreshTokenValidity:       p.GetRefreshTokenValidity(),
	}
}

// proxyEquivalent reports whether two provider states are the same in the
// fields this operator manages.
func proxyEquivalent(a, b *api.ProxyProvider) bool {
	if a == nil || b == nil {
		return a == b
	}
	return proxyStateOf(a) == proxyStateOf(b)
}
