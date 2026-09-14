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
	"encoding/json"
	"fmt"
	"slices"

	api "goauthentik.io/api/v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// outpostPageSize bounds a name lookup. An installation has a handful of
// outposts, so one page is always enough to spot a duplicate name.
const outpostPageSize = 100

// outpostAdapter implements RemoteAdapter for an authentik outpost.
//
// Every reference the outpost needs is resolved by the controller before the
// adapter is built, so the adapter itself never has to decide what to do about
// a reference that is not ready yet.
type outpostAdapter struct {
	client  authentik.Client
	outpost *authentikv1alpha1.Outpost

	// providerIDs are the authentik primary keys of every referenced provider.
	// The controller guarantees this is the complete set: see
	// OutpostReconciler.resolveProviderIDs.
	providerIDs []int32

	// serviceConnection is the resolved service connection UUID, or empty when
	// authentik should not manage the outpost's deployment.
	serviceConnection string

	// observed holds the outpost as last read from authentik, so the
	// controller can reach its token identifier without re-fetching.
	observed *api.Outpost
}

func (a *outpostAdapter) Kind() string { return "Outpost" }

func (a *outpostAdapter) DesiredName() string { return a.outpost.OutpostName() }

func (a *outpostAdapter) Exists(ctx context.Context, id string) (bool, error) {
	const op = "retrieve outpost"

	found, resp, err := a.client.API().OutpostsAPI.OutpostsInstancesRetrieve(ctx, id).Execute()
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

func (a *outpostAdapter) FindByName(ctx context.Context) (string, error) {
	const op = "list outposts"
	name := a.DesiredName()

	list, resp, err := a.client.API().OutpostsAPI.OutpostsInstancesList(ctx).
		NameIexact(name).PageSize(outpostPageSize).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	if list == nil {
		return "", authentik.NotFound(op, "Outpost", name)
	}

	// NameIexact is case-insensitive, so narrow to an exact match client-side:
	// adopting "Grafana" for a resource that declares "grafana" would quietly
	// take over a different outpost.
	var matches []api.Outpost
	for i := range list.Results {
		if list.Results[i].Name == name {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "Outpost", name)
	case 1:
		a.observed = &matches[0]
		return matches[0].Pk, nil
	default:
		return "", authentik.Ambiguous(op, "Outpost", name, len(matches))
	}
}

func (a *outpostAdapter) Create(ctx context.Context) (string, error) {
	const op = "create outpost"

	req, err := a.buildRequest()
	if err != nil {
		return "", err
	}

	created, resp, err := a.client.API().OutpostsAPI.OutpostsInstancesCreate(ctx).
		OutpostRequest(*req).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	a.observed = created
	return created.Pk, nil
}

func (a *outpostAdapter) Update(ctx context.Context, id string) (bool, error) {
	const op = "update outpost"

	req, err := a.buildRequest()
	if err != nil {
		return false, err
	}

	// A full update is used rather than a partial one because the request is
	// built from the complete spec every time; sending it whole keeps the
	// remote object converging on the declared state.
	updated, resp, err := a.client.API().OutpostsAPI.OutpostsInstancesUpdate(ctx, id).
		OutpostRequest(*req).Execute()
	if err != nil {
		return false, authentik.MapResponseError(op, resp, err)
	}

	changed := a.observed == nil || !outpostEquivalent(a.observed, updated)
	a.observed = updated
	return changed, nil
}

func (a *outpostAdapter) Delete(ctx context.Context, id string) error {
	const op = "delete outpost"

	resp, err := a.client.API().OutpostsAPI.OutpostsInstancesDestroy(ctx, id).Execute()
	if err != nil {
		return authentik.MapResponseError(op, resp, err)
	}
	return nil
}

// buildRequest turns the spec, plus the references the controller resolved,
// into an authentik request.
func (a *outpostAdapter) buildRequest() (*api.OutpostRequest, error) {
	config, err := outpostConfig(a.outpost.Spec.Config)
	if err != nil {
		return nil, err
	}

	// providers is never nil: authentik requires the field, and sending the
	// resolved set whole is what makes removing a providerRef take effect.
	providers := a.providerIDs
	if providers == nil {
		providers = []int32{}
	}

	req := api.NewOutpostRequest(
		a.DesiredName(),
		api.OutpostTypeEnum(a.outpost.Spec.Type),
		providers,
		config,
	)

	// The service connection is set or explicitly cleared on every request, so
	// removing serviceConnectionRef from the spec actually detaches it instead
	// of leaving authentik managing a deployment nobody asked for any more.
	if a.serviceConnection != "" {
		req.SetServiceConnection(a.serviceConnection)
	} else {
		req.SetServiceConnectionNil()
	}

	return req, nil
}

// token fetches the API token this outpost authenticates to authentik with.
//
// authentik does not return the token on the outpost itself; the outpost
// carries a token identifier, and the key behind it is read from the tokens
// endpoint. The value is returned to the caller and never logged or recorded
// in status.
func (a *outpostAdapter) token(ctx context.Context) (string, error) {
	const op = "retrieve outpost token"

	if a.observed == nil || a.observed.TokenIdentifier == "" {
		return "", nil
	}

	view, resp, err := a.client.API().CoreAPI.
		CoreTokensViewKeyRetrieve(ctx, a.observed.TokenIdentifier).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	if view == nil {
		return "", nil
	}
	return view.Key, nil
}

// outpostConfig converts the free-form spec config into the shape the
// generated client takes.
//
// A value that will not decode is reported as a rejected spec rather than a
// transient failure: re-sending it would fail identically every time. Only the
// offending key is named, never its value, which may hold a secret an
// administrator chose to place in the outpost configuration.
func outpostConfig(config map[string]apiextensionsv1.JSON) (map[string]any, error) {
	out := make(map[string]any, len(config))
	for key, raw := range config {
		if len(raw.Raw) == 0 {
			out[key] = nil
			continue
		}
		var value any
		if err := json.Unmarshal(raw.Raw, &value); err != nil {
			return nil, &authentik.APIError{
				Op:     "build outpost config",
				Kind:   authentik.ErrValidation,
				Detail: fmt.Sprintf("spec.config key %q does not hold valid JSON", key),
			}
		}
		out[key] = value
	}
	return out, nil
}

// outpostEquivalent reports whether two outpost states are the same in the
// fields this operator manages. Fields authentik computes, such as the token
// identifier and the refresh interval, are ignored so a server-side default
// never looks like drift.
func outpostEquivalent(a, b *api.Outpost) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Type == b.Type &&
		slices.Equal(a.Providers, b.Providers) &&
		a.GetServiceConnection() == b.GetServiceConnection() &&
		configEquivalent(a.Config, b.Config)
}

// configEquivalent compares two outpost configurations by value.
//
// The comparison goes through JSON rather than reflect.DeepEqual because
// authentik round-trips the configuration through JSON, which turns every
// number into a float64; a map compared structurally would otherwise report
// drift on every reconcile.
func configEquivalent(a, b map[string]any) bool {
	left, errLeft := json.Marshal(a)
	right, errRight := json.Marshal(b)
	if errLeft != nil || errRight != nil {
		return false
	}
	return string(left) == string(right)
}
