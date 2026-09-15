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

// OAuth2GrantType is an OAuth2 grant the provider will issue tokens for.
//
// This is a named type rather than a marker on the slice field because
// controller-gen applies an enum marker to the array itself in that case,
// producing a schema that rejects every non-empty list.
//
// The urn: values contain colons, which the marker parser reads as argument
// separators unless each value is quoted.
// +kubebuilder:validation:Enum="authorization_code";"implicit";"hybrid";"refresh_token";"client_credentials";"password";"urn:ietf:params:oauth:grant-type:device_code";"urn:ietf:params:oauth:grant-type:token-exchange"
type OAuth2GrantType string

// RedirectURIMatchingMode controls how a redirect URI is matched.
// +kubebuilder:validation:Enum=strict;regex
type RedirectURIMatchingMode string

// RedirectURI is one permitted OAuth2 redirect target.
type RedirectURI struct {
	// MatchingMode selects exact or regular-expression matching.
	// +kubebuilder:default=strict
	// +optional
	MatchingMode RedirectURIMatchingMode `json:"matchingMode,omitempty"`

	// URL is the redirect URI, or a regular expression matching one.
	// +kubebuilder:validation:MinLength=1
	URL string `json:"url"`
}

// CredentialsSecretRef says where to write the generated client credentials.
type CredentialsSecretRef struct {
	// Name of the Secret to create in this resource's namespace.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ClientIDKey is the Secret key holding the client id.
	// +kubebuilder:default=client-id
	// +optional
	ClientIDKey string `json:"clientIDKey,omitempty"`

	// ClientSecretKey is the Secret key holding the client secret.
	// +kubebuilder:default=client-secret
	// +optional
	ClientSecretKey string `json:"clientSecretKey,omitempty"`

	// IssuerKey optionally holds the provider's issuer URL, which most OIDC
	// clients need alongside the credentials.
	// +kubebuilder:default=issuer
	// +optional
	IssuerKey string `json:"issuerKey,omitempty"`
}

// RotateCredentialsAnnotation triggers a client secret rotation when its value
// changes. Any value works; a timestamp is conventional:
//
//	kubectl annotate oauth2provider grafana \
//	  authentik.k8s.rka.sh/rotate-credentials="$(date -Is)" --overwrite
//
// Consuming workloads need a restart or a secret reloader to pick up the new
// value; the operator cannot do that for them.
const RotateCredentialsAnnotation = "authentik.k8s.rka.sh/rotate-credentials"

// OAuth2ProviderSpec defines an OAuth2/OpenID Connect provider.
type OAuth2ProviderSpec struct {
	ProviderCommonSpec `json:",inline"`

	// ClientType is confidential for clients that can keep a secret, public
	// for those that cannot (SPAs, native apps).
	// +kubebuilder:validation:Enum=confidential;public
	// +kubebuilder:default=confidential
	// +optional
	ClientType string `json:"clientType,omitempty"`

	// GrantTypes the provider will issue tokens for.
	// +optional
	GrantTypes []OAuth2GrantType `json:"grantTypes,omitempty"`

	// ClientID to use. When empty authentik generates one.
	// +optional
	ClientID string `json:"clientID,omitempty"`

	// ClientSecretRef reads a fixed client secret from a Secret in this
	// namespace. When unset, authentik generates a secret.
	//
	// A secret is never accepted inline: putting one in a spec field would
	// store it in plain text in etcd and print it in `kubectl get -o yaml`.
	// +optional
	ClientSecretRef *LocalSecretKeyReference `json:"clientSecretRef,omitempty"`

	// WriteCredentialsTo creates a Secret holding the client id and secret, so
	// the workload that needs them can mount it. This is usually the point of
	// creating the provider in the first place.
	// +optional
	WriteCredentialsTo *CredentialsSecretRef `json:"writeCredentialsTo,omitempty"`

	// RedirectURIs permitted for this client.
	// +optional
	RedirectURIs []RedirectURI `json:"redirectURIs,omitempty"`

	// AccessCodeValidity in authentik duration syntax, e.g. "minutes=1".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	AccessCodeValidity string `json:"accessCodeValidity,omitempty"`

	// AccessTokenValidity in authentik duration syntax, e.g. "hours=1".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	AccessTokenValidity string `json:"accessTokenValidity,omitempty"`

	// RefreshTokenValidity in authentik duration syntax, e.g. "days=30".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	RefreshTokenValidity string `json:"refreshTokenValidity,omitempty"`

	// IncludeClaimsInIDToken embeds scope claims in the id_token, for clients
	// that never call the userinfo endpoint.
	// +optional
	IncludeClaimsInIDToken *bool `json:"includeClaimsInIDToken,omitempty"`

	// SigningKeyPair is the certificate key pair used to sign tokens.
	// +optional
	SigningKeyPair *CertificateKeyPairReference `json:"signingKeyPair,omitempty"`

	// EncryptionKeyPair is the certificate key pair used to encrypt tokens.
	// When set, tokens are returned as JWEs.
	// +optional
	EncryptionKeyPair *CertificateKeyPairReference `json:"encryptionKeyPair,omitempty"`

	// SubMode selects what the `sub` claim contains.
	// +kubebuilder:validation:Enum=hashed_user_id;user_id;user_uuid;user_username;user_email;user_upn
	// +optional
	SubMode string `json:"subMode,omitempty"`

	// IssuerMode selects how the issuer field is built.
	// +kubebuilder:validation:Enum=global;per_provider
	// +optional
	IssuerMode string `json:"issuerMode,omitempty"`

	// LogoutURI is called on logout.
	// +optional
	LogoutURI string `json:"logoutURI,omitempty"`

	// LogoutMethod selects back-channel or front-channel logout.
	// +kubebuilder:validation:Enum=backchannel;frontchannel
	// +optional
	LogoutMethod string `json:"logoutMethod,omitempty"`
}

// OAuth2ProviderStatus reports the provider's state in authentik.
type OAuth2ProviderStatus struct {
	ProviderStatus `json:",inline"`

	// ClientID currently configured in authentik. The client id is not a
	// credential on its own, so it is safe to surface; the secret never is.
	// +optional
	ClientID string `json:"clientID,omitempty"`

	// CredentialsSecretName is the Secret the credentials were written to.
	// +optional
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`

	// CredentialsRotatedAt records the last client secret rotation.
	// +optional
	CredentialsRotatedAt *metav1.Time `json:"credentialsRotatedAt,omitempty"`

	// ObservedRotationToken is the value of the rotation annotation that was
	// last acted on. A rotation happens when the annotation differs from this,
	// which makes the trigger idempotent: re-reconciling the same resource
	// cannot rotate the secret again and break running workloads.
	// +optional
	ObservedRotationToken string `json:"observedRotationToken,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=akoauth2
// +kubebuilder:printcolumn:name="Client ID",type=string,JSONPath=`.status.clientID`
// +kubebuilder:printcolumn:name="Provider",type=integer,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OAuth2Provider manages an OAuth2/OpenID Connect provider in authentik.
type OAuth2Provider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OAuth2ProviderSpec   `json:"spec,omitempty"`
	Status OAuth2ProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OAuth2ProviderList contains a list of OAuth2Provider.
type OAuth2ProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OAuth2Provider `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (p *OAuth2Provider) ConnectionRef() ConnectionReference { return p.Spec.ConnectionRef }

// DeletionPolicy implements controller.ManagedObject.
func (p *OAuth2Provider) DeletionPolicy() DeletionPolicy { return p.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (p *OAuth2Provider) AdoptionPolicy() AdoptionPolicy { return p.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (p *OAuth2Provider) ManagedStatus() *ManagedResourceStatus {
	return &p.Status.ManagedResourceStatus
}

// ProviderName is the name the provider carries in authentik, defaulting to
// the resource name when the spec does not override it.
func (p *OAuth2Provider) ProviderName() string {
	if p.Spec.Name != "" {
		return p.Spec.Name
	}
	return p.Name
}

func init() {
	registerTypes(&OAuth2Provider{}, &OAuth2ProviderList{})
}
