# API reference

Complete field-level reference for every kind in the `authentik.k8s.rka.sh/v1alpha1`
API group.

!!! danger "Do not edit this page by hand"

    Everything between the generated markers below is produced from the Go types
    in `api/v1alpha1/*.go`. Hand edits are overwritten by the next run and fail
    `hack/gen-docs.py --check` in between.

    To change what appears here, **edit the doc comments on the Go types** and
    regenerate. A field's documentation is the comment above it; its validation
    is its `+kubebuilder` markers.

## Regenerating

```sh
make docs-api
```

`make verify` fails if the generated block is stale, so this page cannot drift
from the types. The types themselves are short and thoroughly commented, and
the cluster serves the same schema:

```sh
kubectl explain oauth2provider.spec --recursive
kubectl explain authentikconnection.spec.tokenSecretRef
```

## Kinds

<!-- BEGIN IMPLEMENTATION-STATUS -->
| Kind                          | Scope      | Short names  |
| ----------------------------- | ---------- | ------------ |
| `ClusterAuthentikConnection`  | Cluster    | `clakconn`   |
| `Application`                 | Namespaced | `akapp`      |
| `AuthentikConnection`         | Namespaced | `akconn`     |
| `CertificateKeyPair`          | Namespaced | `akkeypair`  |
| `DockerServiceConnection`     | Namespaced | `akdockersc` |
| `Flow`                        | Namespaced | `akflow`     |
| `KubernetesServiceConnection` | Namespaced | `akk8ssc`    |
| `OAuth2Provider`              | Namespaced | `akoauth2`   |
| `Outpost`                     | Namespaced | `akoutpost`  |
| `PropertyMapping`             | Namespaced | `akmapping`  |
| `ProxyProvider`               | Namespaced | `akproxy`    |
| `SAMLProvider`                | Namespaced | `aksaml`     |
<!-- END IMPLEMENTATION-STATUS -->

Shared types used across kinds — `ConnectionReference`, `LocalSecretKeyReference`,
`SecretKeyReference`, `AdoptionPolicy`, `DeletionPolicy` and
`ManagedResourceStatus` — live in `api/v1alpha1/common_types.go`. The prose
guides describe them: [Connections](../guides/connections.md),
[Providers](../guides/providers.md), [Applications](../guides/applications.md),
[Outposts](../guides/outposts.md).

## Generated reference

<!-- BEGIN GENERATED: api -->

## Packages
- [authentik.k8s.rka.sh/v1alpha1](#authentikk8srkashv1alpha1)


## authentik.k8s.rka.sh/v1alpha1

Package v1alpha1 contains API Schema definitions for the authentik v1alpha1 API group.

### Resource Types
- [Application](#application)
- [AuthentikConnection](#authentikconnection)
- [CertificateKeyPair](#certificatekeypair)
- [CertificateKeyPairList](#certificatekeypairlist)
- [ClusterAuthentikConnection](#clusterauthentikconnection)
- [DockerServiceConnection](#dockerserviceconnection)
- [Flow](#flow)
- [FlowList](#flowlist)
- [KubernetesServiceConnection](#kubernetesserviceconnection)
- [OAuth2Provider](#oauth2provider)
- [Outpost](#outpost)
- [PropertyMapping](#propertymapping)
- [PropertyMappingList](#propertymappinglist)
- [ProxyProvider](#proxyprovider)
- [SAMLProvider](#samlprovider)



#### AdoptedObjectStatus



AdoptedObjectStatus is the status shared by the adopt-only kinds.



_Appears in:_
- [CertificateKeyPair](#certificatekeypair)
- [Flow](#flow)
- [PropertyMapping](#propertymapping)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's UUID for the resolved object. References are<br />resolved through this, so Kubernetes stays the source of truth and a<br />rename inside authentik cannot silently repoint a provider. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. |  | Optional: \{\} <br /> |
| `authentikURL` _string_ | AuthentikURL is the instance this object resolved against.<br />A UUID only means anything on the instance that issued it, so a<br />cross-namespace reference compares this against the referring resource's<br />own connection and refuses a mismatch. Without the check, pointing at a<br />flow from a namespace wired to a different authentik would send a UUID<br />that instance has never seen. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the object was last resolved successfully. |  | Optional: \{\} <br /> |


#### AdoptionPolicy

_Underlying type:_ _string_

AdoptionPolicy controls what happens when an authentik object with the same
name or slug already exists.

authentik objects are keyed by name/slug rather than by Kubernetes UID, so
collisions are routine rather than exceptional. The default refuses to take
over an existing object, because silent adoption is how an operator quietly
overwrites something a human is maintaining by hand.

_Validation:_
- Enum: [FailOnConflict AdoptExisting]

_Appears in:_
- [ApplicationSpec](#applicationspec)
- [DockerServiceConnectionSpec](#dockerserviceconnectionspec)
- [KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)
- [OutpostSpec](#outpostspec)
- [ProviderCommonSpec](#providercommonspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)
- [ServiceConnectionCommonSpec](#serviceconnectioncommonspec)

| Field | Description |
| --- | --- |
| `FailOnConflict` | AdoptionPolicyFailOnConflict refuses to manage a pre-existing object.<br /> |
| `AdoptExisting` | AdoptionPolicyAdoptExisting takes ownership of a pre-existing object.<br /> |


#### Application



Application manages an application in authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `Application` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ApplicationSpec](#applicationspec)_ |  |  |  |
| `status` _[ApplicationStatus](#applicationstatus)_ |  |  |  |


#### ApplicationPolicyEngineMode

_Underlying type:_ _string_

ApplicationPolicyEngineMode selects how several policies bound to one
application are combined.

_Validation:_
- Enum: [all any]

_Appears in:_
- [ApplicationSpec](#applicationspec)



#### ApplicationSpec



ApplicationSpec defines an application in authentik.



_Appears in:_
- [Application](#application)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this application lives in. |  |  |
| `name` _string_ | Name is the application's display name, shown on the user library page.<br />Defaults to the resource name. |  | MaxLength: 255 <br />Optional: \{\} <br /> |
| `slug` _string_ | Slug is the application's internal name, used in its URLs.<br />It is immutable. authentik keys an application by its slug, so changing<br />it cannot be an update: the operator would have to delete the old<br />application and create a new one, which silently discards every policy<br />binding attached to it. Create a new Application instead. |  | MaxLength: 50 <br />MinLength: 1 <br />Pattern: `^[-a-zA-Z0-9_]+$` <br /> |
| `providerRef` _[ProviderReference](#providerreference)_ | ProviderRef is the provider that authenticates users for this<br />application. Leave unset for an application that only appears in the<br />user library and is not itself protected. |  | Optional: \{\} <br /> |
| `backchannelProviderRefs` _[ProviderReference](#providerreference) array_ | BackchannelProviderRefs are additional providers attached to this<br />application for back-channel use, such as SCIM provisioning or an LDAP<br />bind, alongside the primary provider that handles the login itself. |  | Optional: \{\} <br /> |
| `openInNewTab` _boolean_ | OpenInNewTab opens the launch URL in a new browser tab or window. |  | Optional: \{\} <br /> |
| `metaLaunchUrl` _string_ | MetaLaunchURL is the address the library entry links to. Leave unset to<br />let authentik derive it from the provider. |  | Optional: \{\} <br /> |
| `metaIcon` _string_ | MetaIcon is the URL of the icon shown on the library entry.<br />Only a URL is accepted. authentik can also serve an icon uploaded to its<br />own media storage, and that is out of scope for this resource: the file<br />would have to travel through the custom resource as base64 and be<br />re-uploaded on every reconcile, which does not belong in etcd. Host the<br />image somewhere and point at it. |  | Optional: \{\} <br /> |
| `metaDescription` _string_ | MetaDescription is the short description shown on the library entry. |  | Optional: \{\} <br /> |
| `metaPublisher` _string_ | MetaPublisher names the application's publisher on the library entry. |  | Optional: \{\} <br /> |
| `metaHide` _boolean_ | MetaHide keeps the application off the user's library page while leaving<br />it usable. Useful for an application reached only by a direct link. |  | Optional: \{\} <br /> |
| `group` _string_ | Group names the section the application is filed under on the library<br />page. Applications sharing a group are shown together. |  | Optional: \{\} <br /> |
| `policyEngineMode` _[ApplicationPolicyEngineMode](#applicationpolicyenginemode)_ | PolicyEngineMode selects whether every policy bound to this application<br />must pass, or any one of them. | any | Enum: [all any] <br />Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when an application with this slug<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik application when<br />this resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


#### ApplicationStatus



ApplicationStatus reports the application's state in authentik.



_Appears in:_
- [Application](#application)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is the numeric primary key the primary provider reference<br />resolved to. It is surfaced so a mis-wired reference can be diagnosed<br />without reading the provider resource as well. |  | Optional: \{\} <br /> |
| `backchannelProviderIDs` _integer array_ | BackchannelProviderIDs are the numeric primary keys the back-channel<br />provider references resolved to, in spec order. |  | Optional: \{\} <br /> |
| `launchURL` _string_ | LaunchURL is the address authentik currently resolves the library entry<br />to, whether taken from metaLaunchUrl or derived from the provider. |  | Optional: \{\} <br /> |


#### AuthentikConnection



AuthentikConnection describes how to reach one authentik instance, using an
API token stored in the same namespace.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `AuthentikConnection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[AuthentikConnectionSpec](#authentikconnectionspec)_ |  |  |  |
| `status` _[AuthentikConnectionStatus](#authentikconnectionstatus)_ |  |  |  |


#### AuthentikConnectionSpec



AuthentikConnectionSpec defines a connection to an authentik instance, using
credentials held in the same namespace.



_Appears in:_
- [AuthentikConnection](#authentikconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the base URL of the authentik instance, for example<br />https://authentik.example.com. Do not include the /api/v3 suffix. |  | MinLength: 1 <br />Pattern: `^https?://` <br /> |
| `insecureSkipTLSVerify` _boolean_ | InsecureSkipTLSVerify disables verification of the authentik server's TLS<br />certificate. Intended for local testing against a self-signed instance;<br />prefer caBundleSecretRef anywhere else. | false | Optional: \{\} <br /> |
| `cluster` _string_ | Cluster identifies this operator instance when several of them share one<br />authentik - for example one per Kubernetes cluster, or one reaching the<br />instance directly and another through a proxy.<br />authentik has no ownership marker on most objects, so when this is set<br />the operator scopes the names of objects it manages with it: a provider<br />named "grafana" becomes "grafana-prod-eu". Two clusters then get two<br />distinct objects instead of fighting over one, and neither operator can<br />adopt or delete the other's.<br />Leave it unset when only one operator talks to the instance. Changing it<br />later orphans the objects created under the old value; they are not<br />renamed, and the operator will create new ones. |  | MaxLength: 40 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `probeInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#duration-v1-meta)_ | ProbeInterval is how often the connection is re-checked for reachability. | 5m | Pattern: `^([0-9]+(s\|m\|h))+$` <br />Type: string <br />Optional: \{\} <br /> |
| `tokenSecretRef` _[LocalSecretKeyReference](#localsecretkeyreference)_ | TokenSecretRef points at a Secret in this object's own namespace holding<br />an authentik API token. |  |  |
| `caBundleSecretRef` _[LocalSecretKeyReference](#localsecretkeyreference)_ | CABundleSecretRef optionally points at a Secret in this object's own<br />namespace holding a PEM CA bundle used to verify the authentik server. |  | Optional: \{\} <br /> |


#### AuthentikConnectionStatus



AuthentikConnectionStatus reports reachability and version compatibility.



_Appears in:_
- [AuthentikConnection](#authentikconnection)
- [ClusterAuthentikConnection](#clusterauthentikconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the connection. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `authentikVersion` _string_ | AuthentikVersion is the version reported by the instance, e.g. "2026.8.2". |  | Optional: \{\} <br /> |
| `versionSupported` _boolean_ | VersionSupported reports whether AuthentikVersion falls within the range<br />this operator is tested against. When false, dependent resources refuse to<br />reconcile rather than failing obscurely deep inside an API call. |  | Optional: \{\} <br /> |
| `lastProbeTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastProbeTime is when the instance was last contacted. |  | Optional: \{\} <br /> |


#### CertificateKeyPair



CertificateKeyPair identifies an authentik certificate-key pair that
providers can reference.



_Appears in:_
- [CertificateKeyPairList](#certificatekeypairlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `CertificateKeyPair` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[CertificateKeyPairSpec](#certificatekeypairspec)_ |  |  |  |
| `status` _[AdoptedObjectStatus](#adoptedobjectstatus)_ |  |  |  |


#### CertificateKeyPairList



CertificateKeyPairList contains a list of CertificateKeyPair.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `CertificateKeyPairList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[CertificateKeyPair](#certificatekeypair) array_ |  |  |  |


#### CertificateKeyPairReference



CertificateKeyPairReference points at a CertificateKeyPair resource in the
same namespace.



_Appears in:_
- [DockerServiceConnectionSpec](#dockerserviceconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the CertificateKeyPair resource. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `namespace` _string_ | Namespace holding the resource. Defaults to the referring resource's own<br />namespace.<br />Naming another namespace requires the operator to be started with<br />cross-namespace references enabled; otherwise the reference is refused<br />rather than quietly resolved somewhere else. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### CertificateKeyPairSpec



CertificateKeyPairSpec identifies an authentik certificate-key pair.



_Appears in:_
- [CertificateKeyPair](#certificatekeypair)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance holding the key pair. |  |  |
| `existingName` _string_ | ExistingName is the name of a certificate-key pair that already exists<br />in authentik, such as "authentik Self-signed Certificate". |  | MaxLength: 255 <br />MinLength: 1 <br /> |


#### ClusterAuthentikConnection



ClusterAuthentikConnection describes how to reach one authentik instance,
usable from any namespace.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `ClusterAuthentikConnection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ClusterAuthentikConnectionSpec](#clusterauthentikconnectionspec)_ |  |  |  |
| `status` _[AuthentikConnectionStatus](#authentikconnectionstatus)_ |  |  |  |


#### ClusterAuthentikConnectionSpec



ClusterAuthentikConnectionSpec defines a cluster-wide connection to an
authentik instance.

SECURITY: because this object is cluster-scoped it must name the namespace
holding its credentials explicitly, which means it can reference a Secret
anywhere in the cluster. Permission to create or edit one of these is
therefore close to a cluster-admin privilege, and RBAC for it should be
granted accordingly. See SECURITY.md.



_Appears in:_
- [ClusterAuthentikConnection](#clusterauthentikconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the base URL of the authentik instance, for example<br />https://authentik.example.com. Do not include the /api/v3 suffix. |  | MinLength: 1 <br />Pattern: `^https?://` <br /> |
| `insecureSkipTLSVerify` _boolean_ | InsecureSkipTLSVerify disables verification of the authentik server's TLS<br />certificate. Intended for local testing against a self-signed instance;<br />prefer caBundleSecretRef anywhere else. | false | Optional: \{\} <br /> |
| `cluster` _string_ | Cluster identifies this operator instance when several of them share one<br />authentik - for example one per Kubernetes cluster, or one reaching the<br />instance directly and another through a proxy.<br />authentik has no ownership marker on most objects, so when this is set<br />the operator scopes the names of objects it manages with it: a provider<br />named "grafana" becomes "grafana-prod-eu". Two clusters then get two<br />distinct objects instead of fighting over one, and neither operator can<br />adopt or delete the other's.<br />Leave it unset when only one operator talks to the instance. Changing it<br />later orphans the objects created under the old value; they are not<br />renamed, and the operator will create new ones. |  | MaxLength: 40 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `probeInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#duration-v1-meta)_ | ProbeInterval is how often the connection is re-checked for reachability. | 5m | Pattern: `^([0-9]+(s\|m\|h))+$` <br />Type: string <br />Optional: \{\} <br /> |
| `tokenSecretRef` _[SecretKeyReference](#secretkeyreference)_ | TokenSecretRef points at a Secret holding an authentik API token. The<br />namespace is required and is the only place the token is read from; the<br />namespace of a resource referring to this connection is never consulted. |  |  |
| `caBundleSecretRef` _[SecretKeyReference](#secretkeyreference)_ | CABundleSecretRef optionally points at a Secret holding a PEM CA bundle<br />used to verify the authentik server. |  | Optional: \{\} <br /> |
| `allowedNamespaces` _string array_ | AllowedNamespaces optionally restricts which namespaces may reference this<br />connection. An empty list means every namespace may use it.<br />Without this, any user who can create a resource in any namespace can<br />drive an authentik instance they were never granted access to. |  | Optional: \{\} <br /> |


#### ConnectionKind

_Underlying type:_ _string_

ConnectionKind selects which connection CRD a ConnectionReference points at.

_Validation:_
- Enum: [AuthentikConnection ClusterAuthentikConnection]

_Appears in:_
- [ConnectionReference](#connectionreference)

| Field | Description |
| --- | --- |
| `AuthentikConnection` | ConnectionKindNamespaced refers to a namespaced AuthentikConnection.<br /> |
| `ClusterAuthentikConnection` | ConnectionKindCluster refers to a cluster-scoped ClusterAuthentikConnection.<br /> |


#### ConnectionReference



ConnectionReference points at the authentik instance a resource belongs to.

A namespaced AuthentikConnection is always resolved in the referring
resource's own namespace. Cross-namespace references are deliberately not
supported: they would let anyone who can create a resource in one namespace
borrow credentials from another. Use a ClusterAuthentikConnection when a
connection genuinely needs to be shared cluster-wide.



_Appears in:_
- [ApplicationSpec](#applicationspec)
- [CertificateKeyPairSpec](#certificatekeypairspec)
- [DockerServiceConnectionSpec](#dockerserviceconnectionspec)
- [FlowSpec](#flowspec)
- [KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)
- [OutpostSpec](#outpostspec)
- [PropertyMappingSpec](#propertymappingspec)
- [ProviderCommonSpec](#providercommonspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)
- [ServiceConnectionCommonSpec](#serviceconnectioncommonspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _[ConnectionKind](#connectionkind)_ | Kind of connection object being referenced. | AuthentikConnection | Enum: [AuthentikConnection ClusterAuthentikConnection] <br />Optional: \{\} <br /> |
| `name` _string_ | Name of the connection object. |  | MinLength: 1 <br /> |


#### ConnectionSettings



ConnectionSettings holds the transport configuration shared by the namespaced
and cluster-scoped connection kinds.



_Appears in:_
- [AuthentikConnectionSpec](#authentikconnectionspec)
- [ClusterAuthentikConnectionSpec](#clusterauthentikconnectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the base URL of the authentik instance, for example<br />https://authentik.example.com. Do not include the /api/v3 suffix. |  | MinLength: 1 <br />Pattern: `^https?://` <br /> |
| `insecureSkipTLSVerify` _boolean_ | InsecureSkipTLSVerify disables verification of the authentik server's TLS<br />certificate. Intended for local testing against a self-signed instance;<br />prefer caBundleSecretRef anywhere else. | false | Optional: \{\} <br /> |
| `cluster` _string_ | Cluster identifies this operator instance when several of them share one<br />authentik - for example one per Kubernetes cluster, or one reaching the<br />instance directly and another through a proxy.<br />authentik has no ownership marker on most objects, so when this is set<br />the operator scopes the names of objects it manages with it: a provider<br />named "grafana" becomes "grafana-prod-eu". Two clusters then get two<br />distinct objects instead of fighting over one, and neither operator can<br />adopt or delete the other's.<br />Leave it unset when only one operator talks to the instance. Changing it<br />later orphans the objects created under the old value; they are not<br />renamed, and the operator will create new ones. |  | MaxLength: 40 <br />Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$` <br />Optional: \{\} <br /> |
| `probeInterval` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#duration-v1-meta)_ | ProbeInterval is how often the connection is re-checked for reachability. | 5m | Pattern: `^([0-9]+(s\|m\|h))+$` <br />Type: string <br />Optional: \{\} <br /> |


#### CredentialsSecretRef



CredentialsSecretRef says where to write the generated client credentials.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret to create in this resource's namespace. |  | MinLength: 1 <br /> |
| `clientIDKey` _string_ | ClientIDKey is the Secret key holding the client id. | client-id | Optional: \{\} <br /> |
| `clientSecretKey` _string_ | ClientSecretKey is the Secret key holding the client secret. | client-secret | Optional: \{\} <br /> |
| `issuerKey` _string_ | IssuerKey optionally holds the provider's issuer URL, which most OIDC<br />clients need alongside the credentials. | issuer | Optional: \{\} <br /> |


#### DeletionPolicy

_Underlying type:_ _string_

DeletionPolicy controls what happens to the authentik object when the
Kubernetes resource that manages it is deleted.

_Validation:_
- Enum: [Delete Orphan]

_Appears in:_
- [ApplicationSpec](#applicationspec)
- [DockerServiceConnectionSpec](#dockerserviceconnectionspec)
- [KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)
- [OutpostSpec](#outpostspec)
- [ProviderCommonSpec](#providercommonspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)
- [ServiceConnectionCommonSpec](#serviceconnectioncommonspec)

| Field | Description |
| --- | --- |
| `Delete` | DeletionPolicyDelete removes the authentik object along with the resource.<br /> |
| `Orphan` | DeletionPolicyOrphan leaves the authentik object in place.<br /> |


#### DockerServiceConnection



DockerServiceConnection manages a Docker outpost service connection in
authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `DockerServiceConnection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[DockerServiceConnectionSpec](#dockerserviceconnectionspec)_ |  |  |  |
| `status` _[DockerServiceConnectionStatus](#dockerserviceconnectionstatus)_ |  |  |  |


#### DockerServiceConnectionSpec



DockerServiceConnectionSpec defines a Docker service connection.

Either authentik talks to the Docker socket it is mounted with, in which
case local is set, or it dials an explicit URL. Setting both is rejected,
because it hides which of the two endpoints is actually in use.



_Appears in:_
- [DockerServiceConnection](#dockerserviceconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this service connection<br />lives in. |  |  |
| `name` _string_ | Name is the service connection's name in authentik. Defaults to the<br />resource name. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a service connection with this<br />name already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik service connection<br />when this resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |
| `local` _boolean_ | Local makes authentik use the Docker socket mounted into its own<br />container instead of dialling a URL. |  | Optional: \{\} <br /> |
| `url` _string_ | URL of the Docker daemon, either "unix:///var/run/docker.sock" for a<br />local socket or "https://hostname:2376" for a remote daemon. |  | Optional: \{\} <br /> |
| `tlsVerification` _[CertificateKeyPairReference](#certificatekeypairreference)_ | TLSVerification is the certificate key pair holding the CA used to verify<br />the Docker daemon's certificate. |  | Optional: \{\} <br /> |
| `tlsAuthentication` _[CertificateKeyPairReference](#certificatekeypairreference)_ | TLSAuthentication is the certificate key pair presented to the Docker daemon<br />as a client certificate. |  | Optional: \{\} <br /> |


#### DockerServiceConnectionStatus



DockerServiceConnectionStatus reports the connection's state in authentik.



_Appears in:_
- [DockerServiceConnection](#dockerserviceconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `serviceConnectionID` _string_ | ServiceConnectionID is authentik's UUID for this service connection. It<br />is what an Outpost's serviceConnectionRef ultimately resolves to, so it<br />is surfaced separately from the generic RemoteID string. |  | Optional: \{\} <br /> |


#### Flow



Flow identifies an authentik flow that providers can reference.



_Appears in:_
- [FlowList](#flowlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `Flow` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[FlowSpec](#flowspec)_ |  |  |  |
| `status` _[AdoptedObjectStatus](#adoptedobjectstatus)_ |  |  |  |


#### FlowList



FlowList contains a list of Flow.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `FlowList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[Flow](#flow) array_ |  |  |  |


#### FlowReference



FlowReference points at a Flow resource in the same namespace.

Cross-namespace references are deliberately absent, matching every other
reference in this API: they would let anyone who can create a provider in one
namespace borrow configuration from another.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProviderCommonSpec](#providercommonspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Flow resource. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `namespace` _string_ | Namespace holding the resource. Defaults to the referring resource's own<br />namespace.<br />Naming another namespace requires the operator to be started with<br />cross-namespace references enabled; otherwise the reference is refused<br />rather than quietly resolved somewhere else. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### FlowSpec



FlowSpec identifies an authentik flow.



_Appears in:_
- [Flow](#flow)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance holding the flow. |  |  |
| `existingSlug` _string_ | ExistingSlug is the slug of a flow that already exists in authentik,<br />such as "default-provider-authorization-explicit-consent". The operator<br />resolves it and never modifies the flow.<br />A UUID is also accepted and passed through unchanged. |  | MaxLength: 255 <br />MinLength: 1 <br /> |


#### KubernetesServiceConnection



KubernetesServiceConnection manages a Kubernetes outpost service connection
in authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `KubernetesServiceConnection` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)_ |  |  |  |
| `status` _[KubernetesServiceConnectionStatus](#kubernetesserviceconnectionstatus)_ |  |  |  |


#### KubernetesServiceConnectionSpec



KubernetesServiceConnectionSpec defines a Kubernetes service connection.

Either authentik runs inside the cluster it should deploy outposts into, in
which case local is set and its own service account is used, or it needs a
kubeconfig for a remote cluster. Setting both is rejected, because it hides
which of the two credentials is actually in use.



_Appears in:_
- [KubernetesServiceConnection](#kubernetesserviceconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this service connection<br />lives in. |  |  |
| `name` _string_ | Name is the service connection's name in authentik. Defaults to the<br />resource name. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a service connection with this<br />name already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik service connection<br />when this resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |
| `local` _boolean_ | Local makes authentik use the cluster it is itself running in, through<br />its own service account, instead of a kubeconfig. |  | Optional: \{\} <br /> |
| `kubeconfigSecretRef` _[LocalSecretKeyReference](#localsecretkeyreference)_ | KubeconfigSecretRef reads the kubeconfig for a remote cluster from a<br />Secret in this resource's namespace.<br />A kubeconfig is never accepted inline: it is a cluster credential, and a<br />spec field would store it in plain text in etcd and print it in<br />`kubectl get -o yaml`. |  | Optional: \{\} <br /> |
| `verifySSL` _boolean_ | VerifySSL verifies the certificate presented by the Kubernetes API<br />endpoint. Defaults to true in authentik. |  | Optional: \{\} <br /> |


#### KubernetesServiceConnectionStatus



KubernetesServiceConnectionStatus reports the connection's state in
authentik.



_Appears in:_
- [KubernetesServiceConnection](#kubernetesserviceconnection)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `serviceConnectionID` _string_ | ServiceConnectionID is authentik's UUID for this service connection. It<br />is what an Outpost's serviceConnectionRef ultimately resolves to, so it<br />is surfaced separately from the generic RemoteID string. |  | Optional: \{\} <br /> |


#### LocalSecretKeyReference



LocalSecretKeyReference selects one key of a Secret in the same namespace as
the referring object.



_Appears in:_
- [AuthentikConnectionSpec](#authentikconnectionspec)
- [KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  | MinLength: 1 <br /> |
| `key` _string_ | Key within the Secret's data. |  | MinLength: 1 <br /> |


#### ManagedResourceStatus



ManagedResourceStatus is embedded in every authentik-backed resource status.



_Appears in:_
- [ApplicationStatus](#applicationstatus)
- [DockerServiceConnectionStatus](#dockerserviceconnectionstatus)
- [KubernetesServiceConnectionStatus](#kubernetesserviceconnectionstatus)
- [OAuth2ProviderStatus](#oauth2providerstatus)
- [OutpostStatus](#outpoststatus)
- [ProviderStatus](#providerstatus)
- [ProxyProviderStatus](#proxyproviderstatus)
- [SAMLProviderStatus](#samlproviderstatus)
- [ServiceConnectionStatus](#serviceconnectionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |


#### OAuth2GrantType

_Underlying type:_ _string_

OAuth2GrantType is an OAuth2 grant the provider will issue tokens for.

This is a named type rather than a marker on the slice field because
controller-gen applies an enum marker to the array itself in that case,
producing a schema that rejects every non-empty list.

The urn: values contain colons, which the marker parser reads as argument
separators unless each value is quoted.

_Validation:_
- Enum: [authorization_code implicit hybrid refresh_token client_credentials password urn:ietf:params:oauth:grant-type:device_code urn:ietf:params:oauth:grant-type:token-exchange]

_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)



#### OAuth2Provider



OAuth2Provider manages an OAuth2/OpenID Connect provider in authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `OAuth2Provider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OAuth2ProviderSpec](#oauth2providerspec)_ |  |  |  |
| `status` _[OAuth2ProviderStatus](#oauth2providerstatus)_ |  |  |  |


#### OAuth2ProviderSpec



OAuth2ProviderSpec defines an OAuth2/OpenID Connect provider.



_Appears in:_
- [OAuth2Provider](#oauth2provider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this provider lives in. |  |  |
| `name` _string_ | Name is the provider's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `authorizationFlow` _[FlowReference](#flowreference)_ | AuthorizationFlow is the flow used when authorizing this provider. |  |  |
| `invalidationFlow` _[FlowReference](#flowreference)_ | InvalidationFlow is the flow used when ending a session. |  |  |
| `authenticationFlow` _[FlowReference](#flowreference)_ | AuthenticationFlow is the flow used to authenticate a user who reaches<br />the application unauthenticated. Leave unset to use authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _[PropertyMappingReference](#propertymappingreference) array_ | PropertyMappings attached to this provider, in order. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a provider with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik provider when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |
| `clientType` _string_ | ClientType is confidential for clients that can keep a secret, public<br />for those that cannot (SPAs, native apps). | confidential | Enum: [confidential public] <br />Optional: \{\} <br /> |
| `grantTypes` _[OAuth2GrantType](#oauth2granttype) array_ | GrantTypes the provider will issue tokens for. |  | Enum: [authorization_code implicit hybrid refresh_token client_credentials password urn:ietf:params:oauth:grant-type:device_code urn:ietf:params:oauth:grant-type:token-exchange] <br />Optional: \{\} <br /> |
| `clientID` _string_ | ClientID to use. When empty authentik generates one. |  | Optional: \{\} <br /> |
| `clientSecretRef` _[LocalSecretKeyReference](#localsecretkeyreference)_ | ClientSecretRef reads a fixed client secret from a Secret in this<br />namespace. When unset, authentik generates a secret.<br />A secret is never accepted inline: putting one in a spec field would<br />store it in plain text in etcd and print it in `kubectl get -o yaml`. |  | Optional: \{\} <br /> |
| `writeCredentialsTo` _[CredentialsSecretRef](#credentialssecretref)_ | WriteCredentialsTo creates a Secret holding the client id and secret, so<br />the workload that needs them can mount it. This is usually the point of<br />creating the provider in the first place. |  | Optional: \{\} <br /> |
| `redirectURIs` _[RedirectURI](#redirecturi) array_ | RedirectURIs permitted for this client. |  | Optional: \{\} <br /> |
| `accessCodeValidity` _string_ | AccessCodeValidity in authentik duration syntax, e.g. "minutes=1". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `accessTokenValidity` _string_ | AccessTokenValidity in authentik duration syntax, e.g. "hours=1". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `refreshTokenValidity` _string_ | RefreshTokenValidity in authentik duration syntax, e.g. "days=30". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `includeClaimsInIDToken` _boolean_ | IncludeClaimsInIDToken embeds scope claims in the id_token, for clients<br />that never call the userinfo endpoint. |  | Optional: \{\} <br /> |
| `signingKeyPair` _[CertificateKeyPairReference](#certificatekeypairreference)_ | SigningKeyPair is the certificate key pair used to sign tokens. |  | Optional: \{\} <br /> |
| `encryptionKeyPair` _[CertificateKeyPairReference](#certificatekeypairreference)_ | EncryptionKeyPair is the certificate key pair used to encrypt tokens.<br />When set, tokens are returned as JWEs. |  | Optional: \{\} <br /> |
| `subMode` _string_ | SubMode selects what the `sub` claim contains. |  | Enum: [hashed_user_id user_id user_uuid user_username user_email user_upn] <br />Optional: \{\} <br /> |
| `issuerMode` _string_ | IssuerMode selects how the issuer field is built. |  | Enum: [global per_provider] <br />Optional: \{\} <br /> |
| `logoutURI` _string_ | LogoutURI is called on logout. |  | Optional: \{\} <br /> |
| `logoutMethod` _string_ | LogoutMethod selects back-channel or front-channel logout. |  | Enum: [backchannel frontchannel] <br />Optional: \{\} <br /> |


#### OAuth2ProviderStatus



OAuth2ProviderStatus reports the provider's state in authentik.



_Appears in:_
- [OAuth2Provider](#oauth2provider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is authentik's numeric primary key for this provider. It is<br />what an Application or Outpost must reference, so it is surfaced<br />separately from the generic RemoteID string. |  | Optional: \{\} <br /> |
| `clientID` _string_ | ClientID currently configured in authentik. The client id is not a<br />credential on its own, so it is safe to surface; the secret never is. |  | Optional: \{\} <br /> |
| `credentialsSecretName` _string_ | CredentialsSecretName is the Secret the credentials were written to. |  | Optional: \{\} <br /> |
| `credentialsRotatedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | CredentialsRotatedAt records the last client secret rotation. |  | Optional: \{\} <br /> |
| `observedRotationToken` _string_ | ObservedRotationToken is the value of the rotation annotation that was<br />last acted on. A rotation happens when the annotation differs from this,<br />which makes the trigger idempotent: re-reconciling the same resource<br />cannot rotate the secret again and break running workloads. |  | Optional: \{\} <br /> |


#### Outpost



Outpost manages an authentik outpost and the set of providers it serves.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `Outpost` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OutpostSpec](#outpostspec)_ |  |  |  |
| `status` _[OutpostStatus](#outpoststatus)_ |  |  |  |


#### OutpostSpec



OutpostSpec defines an authentik outpost.



_Appears in:_
- [Outpost](#outpost)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this outpost is registered<br />with. |  |  |
| `name` _string_ | Name is the outpost's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `type` _[OutpostType](#outposttype)_ | Type selects which outpost implementation authentik registers. |  | Enum: [proxy ldap radius rac] <br /> |
| `providerRefs` _[ProviderReference](#providerreference) array_ | ProviderRefs are the providers this outpost serves. Every reference must<br />resolve before the outpost is registered or updated. |  | Optional: \{\} <br /> |
| `serviceConnectionRef` _[ServiceConnectionReference](#serviceconnectionreference)_ | ServiceConnectionRef selects the service connection authentik uses to<br />deploy this outpost. Leave unset when you deploy the outpost yourself. |  | Optional: \{\} <br /> |
| `config` _object (keys:string, values:[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#json-v1-apiextensions-k8s-io))_ | Config is passed to authentik verbatim as the outpost's configuration.<br />The schema differs per outpost type and per authentik version, so it is<br />deliberately not modelled here.<br />Well-known keys include: "log_level", "authentik_host",<br />"authentik_host_browser", "authentik_host_insecure",<br />"object_naming_template", "refresh_interval", "kubernetes_replicas",<br />"kubernetes_namespace", "kubernetes_service_type",<br />"kubernetes_ingress_class_name", "kubernetes_ingress_annotations",<br />"kubernetes_ingress_secret_name", "kubernetes_image_pull_secrets",<br />"kubernetes_json_patches", "kubernetes_disabled_components",<br />"docker_network", "docker_map_ports", "docker_labels" and "docker_image".<br />Consult the authentik documentation for the set your version accepts. |  | Optional: \{\} <br /> |
| `writeTokenTo` _[OutpostTokenSecretRef](#outposttokensecretref)_ | WriteTokenTo creates a Secret holding the outpost's API token and the<br />authentik base URL, which is what a self-hosted outpost needs in order to<br />connect back. Leave unset when authentik deploys the outpost itself. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when an outpost with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik outpost when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


#### OutpostStatus



OutpostStatus reports the outpost's state in authentik.



_Appears in:_
- [Outpost](#outpost)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `outpostID` _string_ | OutpostID is authentik's UUID for this outpost. |  | Optional: \{\} <br /> |
| `providerIDs` _integer array_ | ProviderIDs are the authentik primary keys every providerRef resolved<br />to, in spec order. It is empty until all of them resolve. |  | Optional: \{\} <br /> |
| `serviceConnectionID` _string_ | ServiceConnectionID is the UUID serviceConnectionRef resolved to. |  | Optional: \{\} <br /> |
| `tokenIdentifier` _string_ | TokenIdentifier names the authentik token this outpost authenticates<br />with. It is an identifier, not the token itself, which is never placed<br />in status. |  | Optional: \{\} <br /> |
| `tokenSecretName` _string_ | TokenSecretName is the Secret the outpost token was written to. |  | Optional: \{\} <br /> |


#### OutpostTokenSecretRef



OutpostTokenSecretRef says where to write the outpost's connection token.



_Appears in:_
- [OutpostSpec](#outpostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret to create in this resource's namespace. |  | MinLength: 1 <br /> |
| `tokenKey` _string_ | TokenKey is the Secret key holding the outpost API token, which a<br />self-hosted outpost passes as AUTHENTIK_TOKEN. | token | Optional: \{\} <br /> |
| `hostKey` _string_ | HostKey is the Secret key holding the authentik base URL, which a<br />self-hosted outpost passes as AUTHENTIK_HOST. | authentik-host | Optional: \{\} <br /> |


#### OutpostType

_Underlying type:_ _string_

OutpostType selects which authentik outpost implementation to register.

_Validation:_
- Enum: [proxy ldap radius rac]

_Appears in:_
- [OutpostSpec](#outpostspec)

| Field | Description |
| --- | --- |
| `proxy` | OutpostTypeProxy is a forward-auth / reverse proxy outpost.<br /> |
| `ldap` | OutpostTypeLDAP is an LDAP outpost.<br /> |
| `radius` | OutpostTypeRadius is a RADIUS outpost.<br /> |
| `rac` | OutpostTypeRAC is a Remote Access Control outpost.<br /> |


#### PropertyMapping



PropertyMapping identifies an authentik property mapping that providers can
reference.



_Appears in:_
- [PropertyMappingList](#propertymappinglist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `PropertyMapping` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[PropertyMappingSpec](#propertymappingspec)_ |  |  |  |
| `status` _[AdoptedObjectStatus](#adoptedobjectstatus)_ |  |  |  |


#### PropertyMappingList



PropertyMappingList contains a list of PropertyMapping.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `PropertyMappingList` | | |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[PropertyMapping](#propertymapping) array_ |  |  |  |


#### PropertyMappingReference



PropertyMappingReference points at a PropertyMapping resource in the same
namespace.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProviderCommonSpec](#providercommonspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the PropertyMapping resource. |  | MaxLength: 253 <br />MinLength: 1 <br /> |
| `namespace` _string_ | Namespace holding the resource. Defaults to the referring resource's own<br />namespace.<br />Naming another namespace requires the operator to be started with<br />cross-namespace references enabled; otherwise the reference is refused<br />rather than quietly resolved somewhere else. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### PropertyMappingSpec



PropertyMappingSpec identifies an authentik property mapping.



_Appears in:_
- [PropertyMapping](#propertymapping)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance holding the mapping. |  |  |
| `existingName` _string_ | ExistingName is the name of a property mapping that already exists in<br />authentik, such as "authentik default OAuth Mapping: OpenID 'email'". |  | MaxLength: 255 <br />MinLength: 1 <br /> |


#### ProviderCommonSpec



ProviderCommonSpec holds the fields every authentik provider shares.

authentik takes flows, property mappings and certificate key pairs as UUIDs.
The references below name them by slug or name instead and the operator
resolves them, because nobody wants to paste UUIDs into version control. A
value that already looks like a UUID is passed through unchanged.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProxyProviderSpec](#proxyproviderspec)
- [SAMLProviderSpec](#samlproviderspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this provider lives in. |  |  |
| `name` _string_ | Name is the provider's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `authorizationFlow` _[FlowReference](#flowreference)_ | AuthorizationFlow is the flow used when authorizing this provider. |  |  |
| `invalidationFlow` _[FlowReference](#flowreference)_ | InvalidationFlow is the flow used when ending a session. |  |  |
| `authenticationFlow` _[FlowReference](#flowreference)_ | AuthenticationFlow is the flow used to authenticate a user who reaches<br />the application unauthenticated. Leave unset to use authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _[PropertyMappingReference](#propertymappingreference) array_ | PropertyMappings attached to this provider, in order. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a provider with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik provider when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


#### ProviderKind

_Underlying type:_ _string_

ProviderKind selects which provider CRD a ProviderReference points at.

_Validation:_
- Enum: [OAuth2Provider SAMLProvider ProxyProvider]

_Appears in:_
- [ProviderReference](#providerreference)

| Field | Description |
| --- | --- |
| `OAuth2Provider` | ProviderKindOAuth2 refers to an OAuth2Provider.<br /> |
| `SAMLProvider` | ProviderKindSAML refers to a SAMLProvider.<br /> |
| `ProxyProvider` | ProviderKindProxy refers to a ProxyProvider.<br /> |


#### ProviderReference



ProviderReference points at a provider, either one this operator manages in
the same namespace or one that already exists in authentik.

Declared here but shared with Outpost, which references providers the same
way. It lives in this file rather than a shared one only because Application
was the first consumer.

The reference is resolved through the provider resource's own
status.providerID rather than by looking its name up in authentik, so the
Kubernetes objects stay the source of truth and renaming a provider inside
authentik cannot silently repoint an application at something else.

A namespaced provider is always resolved in the application's own namespace.
Cross-namespace references are deliberately not supported: they would let
anyone who can create a referring resource in one namespace attach a
provider, and therefore credentials, owned by another.



_Appears in:_
- [ApplicationSpec](#applicationspec)
- [OutpostSpec](#outpostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _[ProviderKind](#providerkind)_ | Kind of provider resource being referenced. | OAuth2Provider | Enum: [OAuth2Provider SAMLProvider ProxyProvider] <br />Optional: \{\} <br /> |
| `name` _string_ | Name of the provider resource. Mutually exclusive with<br />existingProviderName. |  | MaxLength: 255 <br />Optional: \{\} <br /> |
| `existingProviderName` _string_ | ExistingProviderName is the name of a provider that already exists in<br />authentik and is maintained outside the operator. Use it to attach to a<br />provider somebody else created, rather than one this operator manages. |  | MaxLength: 255 <br />Optional: \{\} <br /> |
| `namespace` _string_ | Namespace holding the resource. Defaults to the referring resource's own<br />namespace.<br />Naming another namespace requires the operator to be started with<br />cross-namespace references enabled; otherwise the reference is refused<br />rather than quietly resolved somewhere else. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### ProviderStatus



ProviderStatus is the status shared by every provider kind.



_Appears in:_
- [OAuth2ProviderStatus](#oauth2providerstatus)
- [ProxyProviderStatus](#proxyproviderstatus)
- [SAMLProviderStatus](#samlproviderstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is authentik's numeric primary key for this provider. It is<br />what an Application or Outpost must reference, so it is surfaced<br />separately from the generic RemoteID string. |  | Optional: \{\} <br /> |


#### ProxyMode

_Underlying type:_ _string_

ProxyMode selects how the outpost serves the application.

_Validation:_
- Enum: [proxy forward_single forward_domain]

_Appears in:_
- [ProxyProviderSpec](#proxyproviderspec)

| Field | Description |
| --- | --- |
| `proxy` | ProxyModeProxy terminates traffic in the outpost and forwards it to an<br />upstream host.<br /> |
| `forward_single` | ProxyModeForwardSingle authorizes one application behind an existing<br />reverse proxy.<br /> |
| `forward_domain` | ProxyModeForwardDomain authorizes every application on a domain behind<br />an existing reverse proxy.<br /> |


#### ProxyProvider



ProxyProvider manages a proxy provider in authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `ProxyProvider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[ProxyProviderSpec](#proxyproviderspec)_ |  |  |  |
| `status` _[ProxyProviderStatus](#proxyproviderstatus)_ |  |  |  |


#### ProxyProviderSpec



ProxyProviderSpec defines a proxy provider served by an authentik outpost.

The forwarding modes have no upstream of their own: an existing reverse
proxy already holds the connection and only asks authentik whether to allow
it. Rejecting the combination here turns a silently ignored field into an
error on apply.
The emptiness test uses size() rather than a comparison against an empty
string literal: a pair of adjacent single quotes inside a comment is
rewritten by gofmt into a typographic quote, which silently corrupts the
rule.



_Appears in:_
- [ProxyProvider](#proxyprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this provider lives in. |  |  |
| `name` _string_ | Name is the provider's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `authorizationFlow` _[FlowReference](#flowreference)_ | AuthorizationFlow is the flow used when authorizing this provider. |  |  |
| `invalidationFlow` _[FlowReference](#flowreference)_ | InvalidationFlow is the flow used when ending a session. |  |  |
| `authenticationFlow` _[FlowReference](#flowreference)_ | AuthenticationFlow is the flow used to authenticate a user who reaches<br />the application unauthenticated. Leave unset to use authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _[PropertyMappingReference](#propertymappingreference) array_ | PropertyMappings attached to this provider, in order. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a provider with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik provider when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |
| `externalHost` _string_ | ExternalHost is the URL the application is reached on by users, for<br />example "https://grafana.example.com". It is what the outpost matches<br />incoming requests against. |  | MinLength: 1 <br /> |
| `internalHost` _string_ | InternalHost is the upstream the outpost forwards traffic to, for<br />example "http://grafana.monitoring.svc.cluster.local:3000". Valid only<br />in proxy mode. |  | Optional: \{\} <br /> |
| `internalHostSSLValidation` _boolean_ | InternalHostSSLValidation verifies the upstream's TLS certificate.<br />Disable it only for an upstream using a self-signed certificate. |  | Optional: \{\} <br /> |
| `mode` _[ProxyMode](#proxymode)_ | Mode selects how the outpost serves the application: proxy terminates<br />traffic and forwards it upstream, while the forward modes authorize<br />requests for an existing reverse proxy. | proxy | Enum: [proxy forward_single forward_domain] <br />Optional: \{\} <br /> |
| `certificate` _[CertificateKeyPairReference](#certificatekeypairreference)_ | Certificate is the certificate key pair presented for the external<br />host. |  | Optional: \{\} <br /> |
| `skipPathRegex` _string_ | SkipPathRegex lists paths that bypass authentication, one regular<br />expression per line. Use it for health checks and public assets.<br />Every request matching one of these expressions reaches the application<br />unauthenticated, so keep the expressions anchored and narrow. |  | Optional: \{\} <br /> |
| `basicAuthEnabled` _boolean_ | BasicAuthEnabled sends HTTP Basic credentials to the upstream, for<br />applications that cannot read authentication headers. |  | Optional: \{\} <br /> |
| `basicAuthUserAttribute` _string_ | BasicAuthUserAttribute is the user attribute holding the username sent<br />as HTTP Basic credentials. |  | Optional: \{\} <br /> |
| `basicAuthPasswordAttribute` _string_ | BasicAuthPasswordAttribute is the user attribute holding the password<br />sent as HTTP Basic credentials. |  | Optional: \{\} <br /> |
| `interceptHeaderAuth` _boolean_ | InterceptHeaderAuth makes the outpost handle Authorization headers sent<br />by the client instead of passing them through to the application. |  | Optional: \{\} <br /> |
| `cookieDomain` _string_ | CookieDomain is the domain the session cookie is issued for. Set it in<br />forward_domain mode so one session covers every application on the<br />domain. |  | Optional: \{\} <br /> |
| `accessTokenValidity` _string_ | AccessTokenValidity in authentik duration syntax, e.g. "hours=24". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `refreshTokenValidity` _string_ | RefreshTokenValidity in authentik duration syntax, e.g. "days=30". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |


#### ProxyProviderStatus



ProxyProviderStatus reports the provider's state in authentik.



_Appears in:_
- [ProxyProvider](#proxyprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is authentik's numeric primary key for this provider. It is<br />what an Application or Outpost must reference, so it is surfaced<br />separately from the generic RemoteID string. |  | Optional: \{\} <br /> |
| `outposts` _string array_ | Outposts names the outposts currently serving this provider. A proxy<br />provider that no outpost serves is unreachable, so an empty list is the<br />usual explanation for an application that cannot be opened. |  | Optional: \{\} <br /> |


#### RedirectURI



RedirectURI is one permitted OAuth2 redirect target.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `matchingMode` _[RedirectURIMatchingMode](#redirecturimatchingmode)_ | MatchingMode selects exact or regular-expression matching. | strict | Enum: [strict regex] <br />Optional: \{\} <br /> |
| `url` _string_ | URL is the redirect URI, or a regular expression matching one. |  | MinLength: 1 <br /> |


#### RedirectURIMatchingMode

_Underlying type:_ _string_

RedirectURIMatchingMode controls how a redirect URI is matched.

_Validation:_
- Enum: [strict regex]

_Appears in:_
- [RedirectURI](#redirecturi)



#### SAMLBinding

_Underlying type:_ _string_

SAMLBinding selects how a SAML message is carried over HTTP.

_Validation:_
- Enum: [redirect post]

_Appears in:_
- [SAMLProviderSpec](#samlproviderspec)



#### SAMLDigestAlgorithm

_Underlying type:_ _string_

SAMLDigestAlgorithm identifies the XML digest algorithm by its W3C URI.

SAML carries algorithms as URIs on the wire, so the URI is what this field
takes; there is no short form.

The values contain colons, which the marker parser reads as argument
separators unless each value is quoted.

_Validation:_
- Enum: [http://www.w3.org/2000/09/xmldsig#sha1 http://www.w3.org/2001/04/xmlenc#sha256 http://www.w3.org/2001/04/xmldsig-more#sha384 http://www.w3.org/2001/04/xmlenc#sha512]

_Appears in:_
- [SAMLProviderSpec](#samlproviderspec)



#### SAMLLogoutMethod

_Underlying type:_ _string_

SAMLLogoutMethod selects how single logout is delivered.

_Validation:_
- Enum: [frontchannel_iframe frontchannel_native backchannel]

_Appears in:_
- [SAMLProviderSpec](#samlproviderspec)



#### SAMLNameIDPolicy

_Underlying type:_ _string_

SAMLNameIDPolicy is the NameID format requested when a service provider does
not ask for one itself.

_Validation:_
- Enum: [urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress urn:oasis:names:tc:SAML:2.0:nameid-format:persistent urn:oasis:names:tc:SAML:1.1:nameid-format:X509SubjectName urn:oasis:names:tc:SAML:2.0:nameid-format:WindowsDomainQualifiedName urn:oasis:names:tc:SAML:2.0:nameid-format:transient urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified]

_Appears in:_
- [SAMLProviderSpec](#samlproviderspec)



#### SAMLProvider



SAMLProvider manages a SAML 2.0 provider in authentik.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `authentik.k8s.rka.sh/v1alpha1` | | |
| `kind` _string_ | `SAMLProvider` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[SAMLProviderSpec](#samlproviderspec)_ |  |  |  |
| `status` _[SAMLProviderStatus](#samlproviderstatus)_ |  |  |  |


#### SAMLProviderSpec



SAMLProviderSpec defines a SAML 2.0 identity provider.



_Appears in:_
- [SAMLProvider](#samlprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this provider lives in. |  |  |
| `name` _string_ | Name is the provider's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `authorizationFlow` _[FlowReference](#flowreference)_ | AuthorizationFlow is the flow used when authorizing this provider. |  |  |
| `invalidationFlow` _[FlowReference](#flowreference)_ | InvalidationFlow is the flow used when ending a session. |  |  |
| `authenticationFlow` _[FlowReference](#flowreference)_ | AuthenticationFlow is the flow used to authenticate a user who reaches<br />the application unauthenticated. Leave unset to use authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _[PropertyMappingReference](#propertymappingreference) array_ | PropertyMappings attached to this provider, in order. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a provider with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik provider when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |
| `acsURL` _string_ | ACSURL is the service provider's Assertion Consumer Service endpoint,<br />where authentik posts the SAML response. |  | MinLength: 1 <br /> |
| `slsURL` _string_ | SLSURL is the service provider's Single Logout Service endpoint. Leave<br />unset to disable single logout. |  | Optional: \{\} <br /> |
| `audience` _string_ | Audience is the intended recipient of the assertion, sent as the<br />AudienceRestriction. Most service providers require it to match their<br />own entity ID. |  | Optional: \{\} <br /> |
| `issuerOverride` _string_ | IssuerOverride replaces the issuer sent in the assertion. Set it when a<br />service provider expects an entity ID other than the one authentik<br />derives from the provider. |  | Optional: \{\} <br /> |
| `assertionValidNotBefore` _string_ | AssertionValidNotBefore is how far in the past an assertion becomes<br />valid, in authentik duration syntax, e.g. "minutes=-5". A negative<br />window absorbs clock skew between authentik and the service provider. |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `assertionValidNotOnOrAfter` _string_ | AssertionValidNotOnOrAfter is how long an assertion stays valid, in<br />authentik duration syntax, e.g. "minutes=5". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `sessionValidNotOnOrAfter` _string_ | SessionValidNotOnOrAfter is how long the session the assertion<br />establishes stays valid, in authentik duration syntax, e.g. "hours=8". |  | Pattern: `^(((microseconds\|milliseconds\|seconds\|minutes\|hours\|days\|weeks)=-?\d+);?)+$` <br />Optional: \{\} <br /> |
| `nameIDMapping` _[PropertyMappingReference](#propertymappingreference)_ | NameIDMapping names a property mapping that produces the NameID value.<br />Leave unset to honour the NameIDPolicy of the incoming request. |  | Optional: \{\} <br /> |
| `authnContextClassRefMapping` _[PropertyMappingReference](#propertymappingreference)_ | AuthnContextClassRefMapping names a property mapping. Configures how the AuthnContextClassRef value is created. Leave unset to derive it from the authentication methods used. |  | Optional: \{\} <br /> |
| `digestAlgorithm` _[SAMLDigestAlgorithm](#samldigestalgorithm)_ | DigestAlgorithm used when signing assertions and responses. |  | Enum: [http://www.w3.org/2000/09/xmldsig#sha1 http://www.w3.org/2001/04/xmlenc#sha256 http://www.w3.org/2001/04/xmldsig-more#sha384 http://www.w3.org/2001/04/xmlenc#sha512] <br />Optional: \{\} <br /> |
| `signatureAlgorithm` _[SAMLSignatureAlgorithm](#samlsignaturealgorithm)_ | SignatureAlgorithm used when signing assertions and responses. It must<br />match the key type of the signing certificate key pair. |  | Enum: [http://www.w3.org/2000/09/xmldsig#rsa-sha1 http://www.w3.org/2001/04/xmldsig-more#rsa-sha256 http://www.w3.org/2001/04/xmldsig-more#rsa-sha384 http://www.w3.org/2001/04/xmldsig-more#rsa-sha512 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512 http://www.w3.org/2000/09/xmldsig#dsa-sha1] <br />Optional: \{\} <br /> |
| `signingKeyPair` _[CertificateKeyPairReference](#certificatekeypairreference)_ | SigningKeyPair is the certificate key pair used to sign outgoing responses. |  | Optional: \{\} <br /> |
| `verificationKeyPair` _[CertificateKeyPairReference](#certificatekeypairreference)_ | VerificationKeyPair is the certificate key pair whose certificate incoming<br />signatures are validated against. Leave unset to accept unsigned requests. |  | Optional: \{\} <br /> |
| `encryptionKeyPair` _[CertificateKeyPairReference](#certificatekeypairreference)_ | EncryptionKeyPair is the certificate key pair used to decrypt incoming<br />assertions. |  | Optional: \{\} <br /> |
| `signAssertion` _boolean_ | SignAssertion signs the assertion element itself. |  | Optional: \{\} <br /> |
| `signResponse` _boolean_ | SignResponse signs the enclosing SAML response element. |  | Optional: \{\} <br /> |
| `signLogoutRequest` _boolean_ | SignLogoutRequest signs logout requests sent to the service provider. |  | Optional: \{\} <br /> |
| `signLogoutResponse` _boolean_ | SignLogoutResponse signs logout responses sent to the service provider. |  | Optional: \{\} <br /> |
| `spBinding` _[SAMLBinding](#samlbinding)_ | SPBinding is the binding used to deliver the response to the service<br />provider's ACS endpoint. |  | Enum: [redirect post] <br />Optional: \{\} <br /> |
| `slsBinding` _[SAMLBinding](#samlbinding)_ | SLSBinding is the binding used to deliver logout messages to the service<br />provider's SLS endpoint. |  | Enum: [redirect post] <br />Optional: \{\} <br /> |
| `logoutMethod` _[SAMLLogoutMethod](#samllogoutmethod)_ | LogoutMethod selects how single logout is delivered. |  | Enum: [frontchannel_iframe frontchannel_native backchannel] <br />Optional: \{\} <br /> |
| `defaultRelayState` _string_ | DefaultRelayState is sent as RelayState when authentik starts the login<br />itself, for service providers that use it to pick a landing page. |  | Optional: \{\} <br /> |
| `defaultNameIDPolicy` _[SAMLNameIDPolicy](#samlnameidpolicy)_ | DefaultNameIDPolicy is the NameID format used when the service provider<br />does not request one. |  | Enum: [urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress urn:oasis:names:tc:SAML:2.0:nameid-format:persistent urn:oasis:names:tc:SAML:1.1:nameid-format:X509SubjectName urn:oasis:names:tc:SAML:2.0:nameid-format:WindowsDomainQualifiedName urn:oasis:names:tc:SAML:2.0:nameid-format:transient urn:oasis:names:tc:SAML:1.1:nameid-format:unspecified] <br />Optional: \{\} <br /> |


#### SAMLProviderStatus



SAMLProviderStatus reports the provider's state in authentik.



_Appears in:_
- [SAMLProvider](#samlprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is authentik's numeric primary key for this provider. It is<br />what an Application or Outpost must reference, so it is surfaced<br />separately from the generic RemoteID string. |  | Optional: \{\} <br /> |
| `metadataURL` _string_ | MetadataURL serves the provider's SAML metadata document, which most<br />service providers can consume directly instead of being configured<br />field by field. |  | Optional: \{\} <br /> |
| `issuerURL` _string_ | IssuerURL is the entity ID authentik presents as the issuer. |  | Optional: \{\} <br /> |


#### SAMLSignatureAlgorithm

_Underlying type:_ _string_

SAMLSignatureAlgorithm identifies the XML signature algorithm by its W3C URI.

_Validation:_
- Enum: [http://www.w3.org/2000/09/xmldsig#rsa-sha1 http://www.w3.org/2001/04/xmldsig-more#rsa-sha256 http://www.w3.org/2001/04/xmldsig-more#rsa-sha384 http://www.w3.org/2001/04/xmldsig-more#rsa-sha512 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha1 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384 http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512 http://www.w3.org/2000/09/xmldsig#dsa-sha1]

_Appears in:_
- [SAMLProviderSpec](#samlproviderspec)



#### SecretKeyReference



SecretKeyReference selects one key of a Secret in an explicitly named
namespace. Used by cluster-scoped objects, which have no namespace of their
own to default to.



_Appears in:_
- [ClusterAuthentikConnectionSpec](#clusterauthentikconnectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  | MinLength: 1 <br /> |
| `namespace` _string_ | Namespace holding the Secret. |  | MinLength: 1 <br /> |
| `key` _string_ | Key within the Secret's data. |  | MinLength: 1 <br /> |


#### ServiceConnectionCommonSpec



ServiceConnectionCommonSpec holds the fields every outpost service
connection shares.

A service connection is what authentik uses to deploy and manage an outpost
on the caller's behalf. It is not a provider, so it carries none of the flow
references providers need; only identity and lifecycle policy are shared.



_Appears in:_
- [DockerServiceConnectionSpec](#dockerserviceconnectionspec)
- [KubernetesServiceConnectionSpec](#kubernetesserviceconnectionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this service connection<br />lives in. |  |  |
| `name` _string_ | Name is the service connection's name in authentik. Defaults to the<br />resource name. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a service connection with this<br />name already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik service connection<br />when this resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


#### ServiceConnectionReference



ServiceConnectionReference points at a service connection resource in the
same namespace.



_Appears in:_
- [OutpostSpec](#outpostspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kubernetesServiceConnectionName` _string_ | KubernetesServiceConnectionName names a KubernetesServiceConnection<br />resource. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `dockerServiceConnectionName` _string_ | DockerServiceConnectionName names a DockerServiceConnection resource. |  | MaxLength: 253 <br />Optional: \{\} <br /> |
| `namespace` _string_ | Namespace holding the resource. Defaults to the referring resource's own<br />namespace.<br />Naming another namespace requires the operator to be started with<br />cross-namespace references enabled; otherwise the reference is refused<br />rather than quietly resolved somewhere else. |  | MaxLength: 63 <br />Optional: \{\} <br /> |


#### ServiceConnectionStatus



ServiceConnectionStatus is the status shared by every service connection
kind.



_Appears in:_
- [DockerServiceConnectionStatus](#dockerserviceconnectionstatus)
- [KubernetesServiceConnectionStatus](#kubernetesserviceconnectionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `serviceConnectionID` _string_ | ServiceConnectionID is authentik's UUID for this service connection. It<br />is what an Outpost's serviceConnectionRef ultimately resolves to, so it<br />is surfaced separately from the generic RemoteID string. |  | Optional: \{\} <br /> |

<!-- END GENERATED: api -->
