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

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OutpostType selects which authentik outpost implementation to register.
// +kubebuilder:validation:Enum=proxy;ldap;radius;rac
type OutpostType string

const (
	// OutpostTypeProxy is a forward-auth / reverse proxy outpost.
	OutpostTypeProxy OutpostType = "proxy"
	// OutpostTypeLDAP is an LDAP outpost.
	OutpostTypeLDAP OutpostType = "ldap"
	// OutpostTypeRadius is a RADIUS outpost.
	OutpostTypeRadius OutpostType = "radius"
	// OutpostTypeRAC is a Remote Access Control outpost.
	OutpostTypeRAC OutpostType = "rac"
)

// OutpostTokenSecretRef says where to write the outpost's connection token.
type OutpostTokenSecretRef struct {
	// Name of the Secret to create in this resource's namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// TokenKey is the Secret key holding the outpost API token, which a
	// self-hosted outpost passes as AUTHENTIK_TOKEN.
	// +kubebuilder:default=token
	// +optional
	TokenKey string `json:"tokenKey,omitempty"`

	// HostKey is the Secret key holding the authentik base URL, which a
	// self-hosted outpost passes as AUTHENTIK_HOST.
	// +kubebuilder:default=authentik-host
	// +optional
	HostKey string `json:"hostKey,omitempty"`
}

// OutpostSpec defines an authentik outpost.
type OutpostSpec struct {
	// ConnectionRef selects the authentik instance this outpost is registered
	// with.
	ConnectionRef ConnectionReference `json:"connectionRef"`

	// Name is the outpost's name in authentik. Defaults to the resource name.
	// +optional
	Name string `json:"name,omitempty"`

	// Type selects which outpost implementation authentik registers.
	Type OutpostType `json:"type"`

	// ProviderRefs are the providers this outpost serves. Every reference must
	// resolve before the outpost is registered or updated.
	// +optional
	ProviderRefs []ProviderReference `json:"providerRefs,omitempty"`

	// ServiceConnectionRef selects the service connection authentik uses to
	// deploy this outpost. Leave unset when you deploy the outpost yourself.
	// +optional
	ServiceConnectionRef *ServiceConnectionReference `json:"serviceConnectionRef,omitempty"`

	// Config is passed to authentik verbatim as the outpost's configuration.
	// The schema differs per outpost type and per authentik version, so it is
	// deliberately not modelled here.
	//
	// Well-known keys include: "log_level", "authentik_host",
	// "authentik_host_browser", "authentik_host_insecure",
	// "object_naming_template", "refresh_interval", "kubernetes_replicas",
	// "kubernetes_namespace", "kubernetes_service_type",
	// "kubernetes_ingress_class_name", "kubernetes_ingress_annotations",
	// "kubernetes_ingress_secret_name", "kubernetes_image_pull_secrets",
	// "kubernetes_json_patches", "kubernetes_disabled_components",
	// "docker_network", "docker_map_ports", "docker_labels" and "docker_image".
	// Consult the authentik documentation for the set your version accepts.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config map[string]apiextensionsv1.JSON `json:"config,omitempty"`

	// WriteTokenTo creates a Secret holding the outpost's API token and the
	// authentik base URL, which is what a self-hosted outpost needs in order to
	// connect back. Leave unset when authentik deploys the outpost itself.
	// +optional
	WriteTokenTo *OutpostTokenSecretRef `json:"writeTokenTo,omitempty"`

	// AdoptionPolicy controls what happens when an outpost with this name
	// already exists in authentik.
	// +kubebuilder:default=FailOnConflict
	// +optional
	Adoption AdoptionPolicy `json:"adoptionPolicy,omitempty"`

	// DeletionPolicy controls what happens to the authentik outpost when this
	// resource is deleted.
	// +kubebuilder:default=Delete
	// +optional
	Deletion DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// OutpostStatus reports the outpost's state in authentik.
type OutpostStatus struct {
	ManagedResourceStatus `json:",inline"`

	// OutpostID is authentik's UUID for this outpost.
	// +optional
	OutpostID string `json:"outpostID,omitempty"`

	// ProviderIDs are the authentik primary keys every providerRef resolved
	// to, in spec order. It is empty until all of them resolve.
	// +optional
	ProviderIDs []int32 `json:"providerIDs,omitempty"`

	// ServiceConnectionID is the UUID serviceConnectionRef resolved to.
	// +optional
	ServiceConnectionID string `json:"serviceConnectionID,omitempty"`

	// TokenIdentifier names the authentik token this outpost authenticates
	// with. It is an identifier, not the token itself, which is never placed
	// in status.
	// +optional
	TokenIdentifier string `json:"tokenIdentifier,omitempty"`

	// TokenSecretName is the Secret the outpost token was written to.
	// +optional
	TokenSecretName string `json:"tokenSecretName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akoutpost
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Outpost",type=string,JSONPath=`.status.outpostID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Outpost manages an authentik outpost and the set of providers it serves.
type Outpost struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OutpostSpec   `json:"spec,omitempty"`
	Status OutpostStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OutpostList contains a list of Outpost.
type OutpostList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Outpost `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (o *Outpost) ConnectionRef() ConnectionReference { return o.Spec.ConnectionRef }

// DeletionPolicy implements controller.ManagedObject.
func (o *Outpost) DeletionPolicy() DeletionPolicy { return o.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (o *Outpost) AdoptionPolicy() AdoptionPolicy { return o.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (o *Outpost) ManagedStatus() *ManagedResourceStatus { return &o.Status.ManagedResourceStatus }

// OutpostName is the name the outpost carries in authentik, defaulting to the
// resource name when the spec does not override it.
func (o *Outpost) OutpostName() string {
	if o.Spec.Name != "" {
		return o.Spec.Name
	}
	return o.Name
}

func init() {
	registerTypes(&Outpost{}, &OutpostList{})
}
