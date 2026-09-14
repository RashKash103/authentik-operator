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

// SAMLDigestAlgorithm identifies the XML digest algorithm by its W3C URI.
//
// SAML carries algorithms as URIs on the wire, so the URI is what this field
// takes; there is no short form.
//
// The values contain colons, which the marker parser reads as argument
// separators unless each value is quoted.
// +kubebuilder:validation:Enum="http://www.w3.org/2000/09/xmldsig#sha1";"http://www.w3.org/2001/04/xmlenc#sha256";"http://www.w3.org/2001/04/xmldsig-more#sha384";"http://www.w3.org/2001/04/xmlenc#sha512"
type SAMLDigestAlgorithm string

// SAMLSignatureAlgorithm identifies the XML signature algorithm by its W3C URI.
// +kubebuilder:validation:Enum="http://www.w3.org/2000/09/xmldsig#rsa-sha1";"http://www.w3.org/2001/04/xmldsig-more#rsa-sha256";"http://www.w3.org/2001/04/xmldsig-more#rsa-sha384";"http://www.w3.org/2001/04/xmldsig-more#rsa-sha512";"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1";"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256";"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384";"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512";"http://www.w3.org/2000/09/xmldsig#dsa-sha1"
type SAMLSignatureAlgorithm string

// SAMLBinding selects how a SAML message is carried over HTTP.
// +kubebuilder:validation:Enum=redirect;post
type SAMLBinding string

// SAMLLogoutMethod selects how single logout is delivered.
// +kubebuilder:validation:Enum=frontchannel_iframe;frontchannel_native;backchannel
type SAMLLogoutMethod string

// SAMLNameIDPolicy is the NameID format requested when a service provider does
// not ask for one itself.
// +kubebuilder:validation:Enum="urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress";"urn:oasis:names:tc:SAML:2.0:nameid-format:persistent";"urn:oasis:names:tc:SAML:1.1:nameid-format:X509SubjectName";"urn:oasis:names:tc:SAML:2.0:nameid-format:WindowsDomainQualifiedName";"urn:oasis:names:tc:SAML:2.0:nameid-format:transient";"urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified"
type SAMLNameIDPolicy string

// SAMLProviderSpec defines a SAML 2.0 identity provider.
type SAMLProviderSpec struct {
	ProviderCommonSpec `json:",inline"`

	// ACSURL is the service provider's Assertion Consumer Service endpoint,
	// where authentik posts the SAML response.
	// +kubebuilder:validation:MinLength=1
	ACSURL string `json:"acsURL"`

	// SLSURL is the service provider's Single Logout Service endpoint. Leave
	// unset to disable single logout.
	// +optional
	SLSURL string `json:"slsURL,omitempty"`

	// Audience is the intended recipient of the assertion, sent as the
	// AudienceRestriction. Most service providers require it to match their
	// own entity ID.
	// +optional
	Audience string `json:"audience,omitempty"`

	// IssuerOverride replaces the issuer sent in the assertion. Set it when a
	// service provider expects an entity ID other than the one authentik
	// derives from the provider.
	// +optional
	IssuerOverride string `json:"issuerOverride,omitempty"`

	// AssertionValidNotBefore is how far in the past an assertion becomes
	// valid, in authentik duration syntax, e.g. "minutes=-5". A negative
	// window absorbs clock skew between authentik and the service provider.
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	AssertionValidNotBefore string `json:"assertionValidNotBefore,omitempty"`

	// AssertionValidNotOnOrAfter is how long an assertion stays valid, in
	// authentik duration syntax, e.g. "minutes=5".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	AssertionValidNotOnOrAfter string `json:"assertionValidNotOnOrAfter,omitempty"`

	// SessionValidNotOnOrAfter is how long the session the assertion
	// establishes stays valid, in authentik duration syntax, e.g. "hours=8".
	// +kubebuilder:validation:Pattern=`^(((microseconds|milliseconds|seconds|minutes|hours|days|weeks)=-?\d+);?)+$`
	// +optional
	SessionValidNotOnOrAfter string `json:"sessionValidNotOnOrAfter,omitempty"`

	// NameIDMapping is the name of the property mapping that produces the
	// NameID. Leave unset to let the requested NameID policy decide.
	// +optional
	NameIDMapping string `json:"nameIDMapping,omitempty"`

	// AuthnContextClassRefMapping is the name of the property mapping that
	// produces the AuthnContextClassRef sent in the assertion.
	// +optional
	AuthnContextClassRefMapping string `json:"authnContextClassRefMapping,omitempty"`

	// DigestAlgorithm used when signing assertions and responses.
	// +optional
	DigestAlgorithm SAMLDigestAlgorithm `json:"digestAlgorithm,omitempty"`

	// SignatureAlgorithm used when signing assertions and responses. It must
	// match the key type of the signing certificate key pair.
	// +optional
	SignatureAlgorithm SAMLSignatureAlgorithm `json:"signatureAlgorithm,omitempty"`

	// SigningKeyPair is the name of the certificate key pair used to sign
	// assertions and responses. Required for the sign* options to take effect.
	// +optional
	SigningKeyPair string `json:"signingKeyPair,omitempty"`

	// VerificationKeyPair is the name of the certificate key pair whose public
	// certificate verifies signed AuthnRequests from the service provider.
	// When set, unsigned requests are rejected.
	// +optional
	VerificationKeyPair string `json:"verificationKeyPair,omitempty"`

	// EncryptionKeyPair is the name of the certificate key pair used to
	// encrypt assertions. When set, assertions are sent encrypted.
	// +optional
	EncryptionKeyPair string `json:"encryptionKeyPair,omitempty"`

	// SignAssertion signs the assertion element itself.
	// +optional
	SignAssertion *bool `json:"signAssertion,omitempty"`

	// SignResponse signs the enclosing SAML response element.
	// +optional
	SignResponse *bool `json:"signResponse,omitempty"`

	// SignLogoutRequest signs logout requests sent to the service provider.
	// +optional
	SignLogoutRequest *bool `json:"signLogoutRequest,omitempty"`

	// SignLogoutResponse signs logout responses sent to the service provider.
	// +optional
	SignLogoutResponse *bool `json:"signLogoutResponse,omitempty"`

	// SPBinding is the binding used to deliver the response to the service
	// provider's ACS endpoint.
	// +optional
	SPBinding SAMLBinding `json:"spBinding,omitempty"`

	// SLSBinding is the binding used to deliver logout messages to the service
	// provider's SLS endpoint.
	// +optional
	SLSBinding SAMLBinding `json:"slsBinding,omitempty"`

	// LogoutMethod selects how single logout is delivered.
	// +optional
	LogoutMethod SAMLLogoutMethod `json:"logoutMethod,omitempty"`

	// DefaultRelayState is sent as RelayState when authentik starts the login
	// itself, for service providers that use it to pick a landing page.
	// +optional
	DefaultRelayState string `json:"defaultRelayState,omitempty"`

	// DefaultNameIDPolicy is the NameID format used when the service provider
	// does not request one.
	// +optional
	DefaultNameIDPolicy SAMLNameIDPolicy `json:"defaultNameIDPolicy,omitempty"`
}

// SAMLProviderStatus reports the provider's state in authentik.
type SAMLProviderStatus struct {
	ProviderStatus `json:",inline"`

	// MetadataURL serves the provider's SAML metadata document, which most
	// service providers can consume directly instead of being configured
	// field by field.
	// +optional
	MetadataURL string `json:"metadataURL,omitempty"`

	// IssuerURL is the entity ID authentik presents as the issuer.
	// +optional
	IssuerURL string `json:"issuerURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=aksaml
// +kubebuilder:printcolumn:name="ACS URL",type=string,JSONPath=`.spec.acsURL`
// +kubebuilder:printcolumn:name="Provider",type=integer,JSONPath=`.status.providerID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SAMLProvider manages a SAML 2.0 provider in authentik.
type SAMLProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SAMLProviderSpec   `json:"spec,omitempty"`
	Status SAMLProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SAMLProviderList contains a list of SAMLProvider.
type SAMLProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SAMLProvider `json:"items"`
}

// ConnectionRef implements controller.ManagedObject.
func (p *SAMLProvider) ConnectionRef() ConnectionReference { return p.Spec.ConnectionRef }

// DeletionPolicy implements controller.ManagedObject.
func (p *SAMLProvider) DeletionPolicy() DeletionPolicy { return p.Spec.Deletion }

// AdoptionPolicy implements controller.ManagedObject.
func (p *SAMLProvider) AdoptionPolicy() AdoptionPolicy { return p.Spec.Adoption }

// ManagedStatus implements controller.ManagedObject.
func (p *SAMLProvider) ManagedStatus() *ManagedResourceStatus {
	return &p.Status.ManagedResourceStatus
}

// ProviderName is the name the provider carries in authentik, defaulting to
// the resource name when the spec does not override it.
func (p *SAMLProvider) ProviderName() string {
	if p.Spec.Name != "" {
		return p.Spec.Name
	}
	return p.Name
}

func init() {
	registerTypes(&SAMLProvider{}, &SAMLProviderList{})
}
