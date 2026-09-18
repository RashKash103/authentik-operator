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

package v1alpha1

// AuthentikDurationPattern matches authentik's duration syntax, for example
// "hours=1;minutes=2;seconds=3" or "days=30".
//
// Validating it at admission turns a confusing API rejection during reconcile
// into an immediate, obvious error on `kubectl apply`.
const AuthentikDurationPattern = `^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`

// ProviderCommonSpec holds the fields every authentik provider shares.
//
// authentik takes flows, property mappings and certificate key pairs as UUIDs.
// The references below name them by slug or name instead and the operator
// resolves them, because nobody wants to paste UUIDs into version control. A
// value that already looks like a UUID is passed through unchanged.
type ProviderCommonSpec struct {
	// ConnectionRef selects the authentik instance this provider lives in.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// Name is the provider's name in authentik. Defaults to the resource name.
	// +optional
	Name string `json:"name,omitempty"`

	// AuthorizationFlow is the flow used when authorizing this provider.
	AuthorizationFlow FlowReference `json:"authorizationFlow"`

	// InvalidationFlow is the flow used when ending a session.
	InvalidationFlow FlowReference `json:"invalidationFlow"`

	// AuthenticationFlow is the flow used to authenticate a user who reaches
	// the application unauthenticated. Leave unset to use authentik's default.
	// +optional
	AuthenticationFlow *FlowReference `json:"authenticationFlow,omitempty"`

	// PropertyMappings attached to this provider, in order.
	// +optional
	PropertyMappings []PropertyMappingReference `json:"propertyMappings,omitempty"`

	// OutpostRefs are the outposts that should serve this provider.
	//
	// The provider names the outposts rather than the outpost naming its
	// providers, so adding one provider is one manifest change. An outpost's
	// membership is reconciled additively: providers attached to it outside
	// this operator are left alone, and removing a reference here detaches only
	// what this operator attached.
	// +optional
	OutpostRefs []OutpostReference `json:"outpostRefs,omitempty"`

	// AdoptionPolicy controls what happens when a provider with this name
	// already exists in authentik.
	// +kubebuilder:default=FailOnConflict
	// +optional
	Adoption AdoptionPolicy `json:"adoptionPolicy,omitempty"`

	// DeletionPolicy controls what happens to the authentik provider when this
	// resource is deleted.
	// +kubebuilder:default=Delete
	// +optional
	Deletion DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// ProviderStatus is the status shared by every provider kind.
type ProviderStatus struct {
	ManagedResourceStatus `json:",inline"`

	// ProviderID is authentik's numeric primary key for this provider. It is
	// what an Application or Outpost must reference, so it is surfaced
	// separately from the generic RemoteID string.
	// +optional
	ProviderID *int32 `json:"providerID,omitempty"`
}

// OutpostReferences returns the outposts this provider says should serve it.
//
// Declared on each concrete type rather than reached through the embedded
// spec, because the outpost controller finds members through an interface
// assertion: a kind that did not implement this would index as having no
// outposts and its providers would silently never be attached.
func (p *OAuth2Provider) OutpostReferences() []OutpostReference { return p.Spec.OutpostRefs }

// OutpostReferences implements the same accessor for SAMLProvider.
func (p *SAMLProvider) OutpostReferences() []OutpostReference { return p.Spec.OutpostRefs }

// OutpostReferences implements the same accessor for ProxyProvider.
func (p *ProxyProvider) OutpostReferences() []OutpostReference { return p.Spec.OutpostRefs }
