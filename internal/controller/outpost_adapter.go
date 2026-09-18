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
	"strconv"

	api "goauthentik.io/api/v3"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	authentikv1alpha1 "rka.sh/authentik-operator/api/v1alpha1"
	"rka.sh/authentik-operator/internal/authentik"
)

// outpostPageSize bounds a name lookup. An installation has a handful of
// outposts, so one page is always enough to spot a duplicate name.
const outpostPageSize = 100

// embeddedOutpostManaged is the marker authentik stamps on the outpost it
// creates for itself.
//
// The embedded outpost is found by this rather than by its display name, which
// is editable and localised - matching on "authentik Embedded Outpost" would
// break the moment somebody renamed it. Confirmed against a live 2026.8
// instance.
const embeddedOutpostManaged = "goauthentik.io/outposts/embedded"

// outpostAdapter implements RemoteAdapter for an authentik outpost.
//
// Every reference the outpost needs is resolved by the controller before the
// adapter is built, so the adapter itself never has to decide what to do about
// a reference that is not ready yet.
type outpostAdapter struct {
	client  authentik.Client
	outpost *authentikv1alpha1.Outpost

	// desiredProviderIDs are the primary keys of the providers that name this
	// outpost. The controller guarantees this is the complete set: see
	// OutpostReconciler.desiredProviderIDs.
	desiredProviderIDs []int32

	// managedProviderIDs is what this operator attached on the previous
	// reconcile, read from status. It is what makes detaching precise: a
	// provider present in authentik but not in this list belongs to somebody
	// else and is left alone.
	managedProviderIDs []int32

	// serviceConnection is the resolved service connection UUID, or empty when
	// authentik should not manage the outpost's deployment.
	serviceConnection string

	// observed holds the outpost as last read from authentik, so the
	// controller can reach its token identifier without re-fetching.
	observed *api.Outpost
}

func (a *outpostAdapter) Kind() string { return "Outpost" }

// referencedDescription names the referenced outpost for an error message.
func (a *outpostAdapter) referencedDescription() string {
	if a.outpost.Spec.Embedded {
		return "the embedded outpost"
	}
	return "outpost " + strconv.Quote(a.outpost.Spec.ExistingOutpostName)
}

// DesiredName is the outpost's name in authentik.
//
// A referenced outpost keeps its name verbatim: cluster scoping distinguishes
// objects this operator creates, and a referenced one belongs to somebody else.
func (a *outpostAdapter) DesiredName() string {
	if a.outpost.ReferencesExisting() {
		return a.outpost.OutpostName()
	}
	return ScopedName(a.client.Cluster(), a.outpost.OutpostName())
}

// attachedProviderIDs is the provider set this reconcile will send: everything
// attached that the operator did not attach, plus everything now desired.
func (a *outpostAdapter) attachedProviderIDs() []int32 {
	var current []int32
	if a.observed != nil {
		current = a.observed.Providers
	}
	return mergeMembership(current, a.managedProviderIDs, a.desiredProviderIDs)
}

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
	if a.outpost.Spec.Embedded {
		return a.findEmbedded(ctx)
	}

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

// findEmbedded locates authentik's built-in outpost by its managed marker.
//
// There is no filter for it, so the outposts are listed and narrowed here. An
// installation has a handful, and authentik creates exactly one embedded
// outpost, so more than one match means something is wrong rather than
// something to choose between.
func (a *outpostAdapter) findEmbedded(ctx context.Context) (string, error) {
	const op = "find embedded outpost"

	list, resp, err := a.client.API().OutpostsAPI.OutpostsInstancesList(ctx).
		PageSize(outpostPageSize).Execute()
	if err != nil {
		return "", authentik.MapResponseError(op, resp, err)
	}
	if list == nil {
		return "", authentik.NotFound(op, "Outpost", embeddedOutpostManaged)
	}

	var matches []api.Outpost
	for i := range list.Results {
		if list.Results[i].GetManaged() == embeddedOutpostManaged {
			matches = append(matches, list.Results[i])
		}
	}

	switch len(matches) {
	case 0:
		return "", authentik.NotFound(op, "Outpost", "the embedded outpost")
	case 1:
		a.observed = &matches[0]
		return matches[0].Pk, nil
	default:
		return "", authentik.Ambiguous(op, "Outpost", "the embedded outpost", len(matches))
	}
}

func (a *outpostAdapter) Create(ctx context.Context) (string, error) {
	const op = "create outpost"

	// A referenced outpost is somebody else's. Reaching here means it was not
	// found, and creating one would quietly produce a second outpost rather
	// than reporting that the one named does not exist - for the embedded
	// outpost, a duplicate of a singleton.
	if a.outpost.ReferencesExisting() {
		return "", fmt.Errorf("%w: %s does not exist in authentik, and is referenced rather than managed, so it will not be created",
			authentik.ErrNotFound, a.referencedDescription())
	}

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
	//
	// For a referenced outpost the request carries the observed type, config
	// and service connection unchanged, so the only field this operator moves
	// is the provider set. See buildRequest.
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
	// providers is never nil: authentik requires the field. The set is merged
	// rather than replaced, so a provider attached outside the operator is not
	// silently detached - for a proxy outpost that would stop protecting the
	// application behind it, with nothing anywhere saying so.
	providers := a.attachedProviderIDs()
	if providers == nil {
		providers = []int32{}
	}

	if a.outpost.ReferencesExisting() {
		return a.buildMembershipRequest(providers)
	}

	config, err := outpostConfig(a.outpost.Spec.Config)
	if err != nil {
		return nil, err
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

// buildMembershipRequest changes the provider set of an outpost this operator
// does not own, and nothing else about it.
//
// authentik updates outposts with a full PUT, so every field the request omits
// is cleared. Building this from the spec the way a managed outpost is built
// would send an empty type and an empty config and wipe the outpost - for the
// embedded outpost, the one authentik ships and every proxy provider hangs off.
// So every other field is carried through from what was just observed.
func (a *outpostAdapter) buildMembershipRequest(providers []int32) (*api.OutpostRequest, error) {
	if len(a.outpost.Spec.Config) > 0 {
		// CEL cannot reach this field, so the check lives here. Accepting it
		// silently would let someone write a config that is never applied.
		return nil, fmt.Errorf("%w: spec.config is not accepted when referencing %s: its configuration is not this operator's to change",
			authentik.ErrValidation, a.referencedDescription())
	}

	if a.observed == nil {
		// Unreachable: Sync observes the outpost before updating it, and Create
		// is refused for a referenced outpost. Guarded anyway, because the
		// failure mode is destroying somebody else's outpost.
		return nil, fmt.Errorf("%w: %s was not read before updating it",
			authentik.ErrValidation, a.referencedDescription())
	}

	req := api.NewOutpostRequest(
		a.observed.Name,
		a.observed.Type,
		providers,
		a.observed.Config,
	)

	// managed is what marks the embedded outpost as authentik's own. Omitting
	// it from a full update clears it, which would leave authentik believing
	// its built-in outpost no longer exists.
	if managed := a.observed.GetManaged(); managed != "" {
		req.SetManaged(managed)
	}

	if sc := a.observed.GetServiceConnection(); sc != "" {
		req.SetServiceConnection(sc)
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
