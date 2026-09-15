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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ProviderKind selects which provider CRD a ProviderReference points at.
// +kubebuilder:validation:Enum=OAuth2Provider;SAMLProvider;ProxyProvider
type ProviderKind string

const (
	// ProviderKindOAuth2 refers to an OAuth2Provider.
	ProviderKindOAuth2 ProviderKind = "OAuth2Provider"
	// ProviderKindSAML refers to a SAMLProvider.
	ProviderKindSAML ProviderKind = "SAMLProvider"
	// ProviderKindProxy refers to a ProxyProvider.
	ProviderKindProxy ProviderKind = "ProxyProvider"
)

// ProviderReference points at a provider, either one this operator manages in
// the same namespace or one that already exists in authentik.
//
// +kubebuilder:validation:XValidation:rule="has(self.name) != has(self.existingProviderName)",message="exactly one of name or existingProviderName must be set"
//
// Declared here but shared with Outpost, which references providers the same
// way. It lives in this file rather than a shared one only because Application
// was the first consumer.
//
// The reference is resolved through the provider resource's own
// status.providerID rather than by looking its name up in authentik, so the
// Kubernetes objects stay the source of truth and renaming a provider inside
// authentik cannot silently repoint an application at something else.
//
// A namespaced provider is always resolved in the application's own namespace.
// Cross-namespace references are deliberately not supported: they would let
// anyone who can create a referring resource in one namespace attach a
// provider, and therefore credentials, owned by another.
type ProviderReference struct {
	// Kind of provider resource being referenced.
	// +kubebuilder:default=OAuth2Provider
	// +optional
	Kind ProviderKind `json:"kind,omitempty"`

	// Name of the provider resource in this namespace. Mutually exclusive with
	// existingProviderName.
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Name string `json:"name,omitempty"`

	// ExistingProviderName is the name of a provider that already exists in
	// authentik and is maintained outside the operator. Use it to attach to a
	// provider somebody else created, rather than one this operator manages.
	// +kubebuilder:validation:MaxLength=255
	// +optional
	ExistingProviderName string `json:"existingProviderName,omitempty"`
}

// Key returns a stable identifier for the reference, used as a field index
// key so an Application can be found from the provider it points at.
func (r ProviderReference) Key() string {
	return string(r.EffectiveKind()) + "/" + r.Name
}

// EffectiveKind returns the referenced kind, applying the OAuth2Provider
// default for an object that predates the CRD default or was built in code.
func (r ProviderReference) EffectiveKind() ProviderKind {
	if r.Kind == "" {
		return ProviderKindOAuth2
	}
	return r.Kind
}

// ApplicationPolicyEngineMode selects how several policies bound to one
// application are combined.
// +kubebuilder:validation:Enum=all;any
type ApplicationPolicyEngineMode string

// ApplicationSpec defines an application in authentik.
type ApplicationSpec struct {
	// ConnectionRef selects the authentik instance this application lives in.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// Name is the application's display name, shown on the user library page.
	// Defaults to the resource name.
	// +kubebuilder:validation:MaxLength=255
	// +optional
	Name string `json:"name,omitempty"`

	// Slug is the application's internal name, used in its URLs.
	//
	// It is immutable. authentik keys an application by its slug, so changing
	// it cannot be an update: the operator would have to delete the old
	// application and create a new one, which silently discards every policy
	// binding attached to it. Create a new Application instead.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=50
	// +kubebuilder:validation:Pattern=`^[-a-zA-Z0-9_]+$`
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="slug is immutable; create a new Application instead"
	Slug string `json:"slug"`

	// ProviderRef is the provider that authenticates users for this
	// application. Leave unset for an application that only appears in the
	// user library and is not itself protected.
	// +optional
	ProviderRef *ProviderReference `json:"providerRef,omitempty"`

	// BackchannelProviderRefs are additional providers attached to this
	// application for back-channel use, such as SCIM provisioning or an LDAP
	// bind, alongside the primary provider that handles the login itself.
	// +optional
	BackchannelProviderRefs []ProviderReference `json:"backchannelProviderRefs,omitempty"`

	// OpenInNewTab opens the launch URL in a new browser tab or window.
	// +optional
	OpenInNewTab *bool `json:"openInNewTab,omitempty"`

	// MetaLaunchURL is the address the library entry links to. Leave unset to
	// let authentik derive it from the provider.
	// +optional
	MetaLaunchURL string `json:"metaLaunchUrl,omitempty"`

	// MetaIcon is the URL of the icon shown on the library entry.
	//
	// Only a URL is accepted. authentik can also serve an icon uploaded to its
	// own media storage, and that is out of scope for this resource: the file
	// would have to travel through the custom resource as base64 and be
	// re-uploaded on every reconcile, which does not belong in etcd. Host the
	// image somewhere and point at it.
	// +optional
	MetaIcon string `json:"metaIcon,omitempty"`

	// MetaDescription is the short description shown on the library entry.
	// +optional
	MetaDescription string `json:"metaDescription,omitempty"`

	// MetaPublisher names the application's publisher on the library entry.
	// +optional
	MetaPublisher string `json:"metaPublisher,omitempty"`

	// MetaHide keeps the application off the user's library page while leaving
	// it usable. Useful for an application reached only by a direct link.
	// +optional
	MetaHide *bool `json:"metaHide,omitempty"`

	// Group names the section the application is filed under on the library
	// page. Applications sharing a group are shown together.
	// +optional
	Group string `json:"group,omitempty"`

	// PolicyEngineMode selects whether every policy bound to this application
	// must pass, or any one of them.
	// +kubebuilder:default=any
	// +optional
	PolicyEngineMode ApplicationPolicyEngineMode `json:"policyEngineMode,omitempty"`

	// AdoptionPolicy controls what happens when an application with this slug
	// already exists in authentik.
	// +kubebuilder:default=FailOnConflict
	// +optional
	Adoption AdoptionPolicy `json:"adoptionPolicy,omitempty"`

	// DeletionPolicy controls what happens to the authentik application when
	// this resource is deleted.
	// +kubebuilder:default=Delete
	// +optional
	Deletion DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// ApplicationStatus reports the application's state in authentik.
type ApplicationStatus struct {
	ManagedResourceStatus `json:",inline"`

	// ProviderID is the numeric primary key the primary provider reference
	// resolved to. It is surfaced so a mis-wired reference can be diagnosed
	// without reading the provider resource as well.
	// +optional
	ProviderID *int32 `json:"providerID,omitempty"`

	// BackchannelProviderIDs are the numeric primary keys the back-channel
	// provider references resolved to, in spec order.
	// +optional
	BackchannelProviderIDs []int32 `json:"backchannelProviderIDs,omitempty"`

	// LaunchURL is the address authentik currently resolves the library entry
	// to, whether taken from metaLaunchUrl or derived from the provider.
	// +optional
	LaunchURL string `json:"launchURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akapp
// +kubebuilder:printcolumn:name="Slug",type=string,JSONPath=`.spec.slug`
// +kubebuilder:printcolumn:name="Provider",type=integer,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Application manages an application in authentik.
type Application struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApplicationSpec   `json:"spec,omitempty"`
	Status ApplicationStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ApplicationList contains a list of Application.
type ApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Application `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (a *Application) ConnectionRef() ConnectionReference { return a.Spec.ConnectionRef }

// DeletionPolicy implements controller.ManagedObject.
func (a *Application) DeletionPolicy() DeletionPolicy { return a.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (a *Application) AdoptionPolicy() AdoptionPolicy { return a.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (a *Application) ManagedStatus() *ManagedResourceStatus {
	return &a.Status.ManagedResourceStatus
}

// ApplicationName is the display name the application carries in authentik,
// defaulting to the resource name when the spec does not override it.
func (a *Application) ApplicationName() string {
	if a.Spec.Name != "" {
		return a.Spec.Name
	}
	return a.Name
}

// ProviderRefs returns every provider reference the spec makes, primary first.
func (a *Application) ProviderRefs() []ProviderReference {
	refs := make([]ProviderReference, 0, len(a.Spec.BackchannelProviderRefs)+1)
	if a.Spec.ProviderRef != nil {
		refs = append(refs, *a.Spec.ProviderRef)
	}
	refs = append(refs, a.Spec.BackchannelProviderRefs...)
	return refs
}

func init() {
	registerTypes(&Application{}, &ApplicationList{})
}
