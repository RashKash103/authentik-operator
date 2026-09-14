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
	"slices"

	api "goauthentik.io/api/v3"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// applicationAdapter implements RemoteAdapter for an authentik application.
//
// Unlike providers, applications are addressed by slug rather than by numeric
// primary key on every endpoint, so the recorded RemoteID is the slug. That is
// only safe because spec.slug is immutable: were it editable, the recorded ID
// would stop matching the object it names.
type applicationAdapter struct {
	client authentik.Client
	app    *authentikv1alpha1.Application

	// providerID and backchannelProviderIDs are resolved by the controller
	// from the referenced provider resources' status. The adapter takes them
	// pre-resolved because resolution reads Kubernetes objects, not authentik.
	providerID             *int32
	backchannelProviderIDs []int32

	// observed holds the application as last read from authentik, so the
	// controller can surface the resolved launch URL without re-fetching.
	observed *api.Application
}

func (a *applicationAdapter) Kind() string { return "Application" }

// DesiredName returns the slug, which is what authentik keys applications by
// and therefore what a pre-existing application would collide on.
func (a *applicationAdapter) DesiredName() string { return a.app.Spec.Slug }

func (a *applicationAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve application"

	found, resp, err := a.client.API().CoreAPI.CoreApplicationsRetrieve(ctx, id).Execute()
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

func (a *applicationAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list applications"
	slug := a.DesiredName()

	// SuperuserFullList is required: the list endpoint filters by policy-based
	// access by default, so without it an application the token's user is not
	// permitted to launch reads as absent and the operator would try to create
	// a duplicate.
	list, resp, err := a.client.API().CoreAPI.CoreApplicationsList(ctx).
		Slug(slug).SuperuserFullList(true).PageSize(100).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}

	// The slug filter is not guaranteed to be exact, so narrow client-side.
	var matches []api.Application
	for i := range list.Results {
		if list.Results[i].Slug == slug {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "application", slug)
	case 1:
		a.observed = &matches[0]
		return matches[0].Slug, nil
	default:
		// authentik enforces slug uniqueness, so this should be unreachable;
		// reporting it is still better than picking one arbitrarily.
		return "", authentik.Ambiguous(op, "application", slug, len(matches))
	}
}

func (a *applicationAdapter) Create(ctx context.Context) (string, error) {
	const op = "create application"

	created, resp, err := a.client.API().CoreAPI.CoreApplicationsCreate(ctx).
		ApplicationRequest(a.buildRequest()).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return created.Slug, nil
}

func (a *applicationAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update application"

	// A full update rather than a partial one: the request is built from the
	// complete spec every time, so sending it whole is what makes a field
	// removed from the spec actually get cleared in authentik.
	updated, resp, err := a.client.API().CoreAPI.CoreApplicationsUpdate(ctx, id).
		ApplicationRequest(a.buildRequest()).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !applicationEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *applicationAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete application"

	resp, err := a.client.API().CoreAPI.CoreApplicationsDestroy(ctx, id).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec, plus the provider keys the controller already
// resolved, into an authentik request.
func (a *applicationAdapter) buildRequest() api.ApplicationRequest {
	spec := a.app.Spec
	req := *api.NewApplicationRequest(a.app.ApplicationName(), spec.Slug)

	if a.providerID != nil {
		req.SetProvider(*a.providerID)
	} else {
		// Explicitly null rather than omitted, so that removing providerRef
		// from the spec detaches the provider instead of leaving the previous
		// one attached forever.
		req.SetProviderNil()
	}

	// Always sent, empty included, so that removing a back-channel reference
	// from the spec detaches it.
	backchannel := a.backchannelProviderIDs
	if backchannel == nil {
		backchannel = []int32{}
	}
	req.SetBackchannelProviders(backchannel)

	setIfNotEmpty(spec.MetaLaunchURL, req.SetMetaLaunchUrl)
	setIfNotEmpty(spec.MetaIcon, req.SetMetaIcon)
	setIfNotEmpty(spec.MetaDescription, req.SetMetaDescription)
	setIfNotEmpty(spec.MetaPublisher, req.SetMetaPublisher)
	setIfNotEmpty(spec.Group, req.SetGroup)
	setIfNotEmpty(string(spec.PolicyEngineMode), func(v string) {
		req.SetPolicyEngineMode(api.PolicyEngineMode(v))
	})
	if spec.OpenInNewTab != nil {
		req.SetOpenInNewTab(*spec.OpenInNewTab)
	}
	if spec.MetaHide != nil {
		req.SetMetaHide(*spec.MetaHide)
	}

	return req
}

// applicationEquivalent reports whether two application states are the same in
// the fields this operator manages. Fields authentik computes, such as the
// resolved launch URL and the expanded provider objects, are ignored so that a
// server-side default never looks like drift.
func applicationEquivalent(a, b *api.Application) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Slug == b.Slug &&
		a.GetProvider() == b.GetProvider() &&
		slices.Equal(a.BackchannelProviders, b.BackchannelProviders) &&
		a.GetOpenInNewTab() == b.GetOpenInNewTab() &&
		a.GetMetaLaunchUrl() == b.GetMetaLaunchUrl() &&
		a.GetMetaIcon() == b.GetMetaIcon() &&
		a.GetMetaDescription() == b.GetMetaDescription() &&
		a.GetMetaPublisher() == b.GetMetaPublisher() &&
		a.GetMetaHide() == b.GetMetaHide() &&
		a.GetGroup() == b.GetGroup() &&
		a.GetPolicyEngineMode() == b.GetPolicyEngineMode()
}
