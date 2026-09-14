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

!!! warning "`make docs-api` does not exist yet"

    This target is planned and is **not** in the Makefile today, so the
    generated block below is empty. It is a stub rather than a stale copy on
    purpose: a hand-written API reference is wrong within a week of the types
    changing, and a wrong reference is worse than a missing one.

    Until the target lands, read the types directly. They are short, thoroughly
    commented, and are the actual source of truth:

    - `api/v1alpha1/authentikconnection_types.go`
    - `api/v1alpha1/clusterauthentikconnection_types.go`
    - `api/v1alpha1/common_types.go`

    Or ask the cluster, which serves the generated OpenAPI schema:

    ```sh
    kubectl explain authentikconnection.spec --recursive
    kubectl explain clusterauthentikconnection.spec.tokenSecretRef
    ```

## What exists today

| Kind | Scope | Short name | API types |
| --- | --- | --- | --- |
| `AuthentikConnection` | Namespaced | `akconn` | Defined |
| `ClusterAuthentikConnection` | Cluster | `clakconn` | Defined |
| `OAuth2Provider` | Namespaced | — | Planned |
| `SAMLProvider` | Namespaced | — | Planned |
| `ProxyProvider` | Namespaced | — | Planned |
| `Application` | Namespaced | — | Planned |
| `Outpost` | Namespaced | — | Planned |
| `KubernetesServiceConnection` | Namespaced | — | Planned |
| `DockerServiceConnection` | Namespaced | — | Planned |

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
- [AuthentikConnection](#authentikconnection)
- [ClusterAuthentikConnection](#clusterauthentikconnection)
- [OAuth2Provider](#oauth2provider)



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
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProviderCommonSpec](#providercommonspec)

| Field | Description |
| --- | --- |
| `FailOnConflict` | AdoptionPolicyFailOnConflict refuses to manage a pre-existing object.<br /> |
| `AdoptExisting` | AdoptionPolicyAdoptExisting takes ownership of a pre-existing object.<br /> |


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
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProviderCommonSpec](#providercommonspec)

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
- [OAuth2ProviderSpec](#oauth2providerspec)
- [ProviderCommonSpec](#providercommonspec)

| Field | Description |
| --- | --- |
| `Delete` | DeletionPolicyDelete removes the authentik object along with the resource.<br /> |
| `Orphan` | DeletionPolicyOrphan leaves the authentik object in place.<br /> |


#### LocalSecretKeyReference



LocalSecretKeyReference selects one key of a Secret in the same namespace as
the referring object.



_Appears in:_
- [AuthentikConnectionSpec](#authentikconnectionspec)
- [OAuth2ProviderSpec](#oauth2providerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name of the Secret. |  | MinLength: 1 <br /> |
| `key` _string_ | Key within the Secret's data. |  | MinLength: 1 <br /> |


#### ManagedResourceStatus



ManagedResourceStatus is embedded in every authentik-backed resource status.



_Appears in:_
- [OAuth2ProviderStatus](#oauth2providerstatus)
- [ProviderStatus](#providerstatus)

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
| `authorizationFlow` _string_ | AuthorizationFlow is the slug of the flow used when authorizing this<br />provider. |  | MinLength: 1 <br /> |
| `invalidationFlow` _string_ | InvalidationFlow is the slug of the flow used when ending a session. |  | MinLength: 1 <br /> |
| `authenticationFlow` _string_ | AuthenticationFlow is the slug of the flow used to authenticate a user<br />who reaches the application unauthenticated. Leave unset to use<br />authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _string array_ | PropertyMappings are the names of property mappings to attach. |  | Optional: \{\} <br /> |
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
| `signingKey` _string_ | SigningKey is the name of the certificate key pair used to sign tokens. |  | Optional: \{\} <br /> |
| `encryptionKey` _string_ | EncryptionKey is the name of the certificate key pair used to encrypt<br />tokens. When set, tokens are returned as JWEs. |  | Optional: \{\} <br /> |
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


#### ProviderCommonSpec



ProviderCommonSpec holds the fields every authentik provider shares.

authentik takes flows, property mappings and certificate key pairs as UUIDs.
These fields accept the human-readable slug or name instead and the operator
resolves them, because nobody wants to paste UUIDs into version control. A
value that already looks like a UUID is passed through unchanged, so either
form works.



_Appears in:_
- [OAuth2ProviderSpec](#oauth2providerspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connectionRef` _[ConnectionReference](#connectionreference)_ | ConnectionRef selects the authentik instance this provider lives in. |  |  |
| `name` _string_ | Name is the provider's name in authentik. Defaults to the resource name. |  | Optional: \{\} <br /> |
| `authorizationFlow` _string_ | AuthorizationFlow is the slug of the flow used when authorizing this<br />provider. |  | MinLength: 1 <br /> |
| `invalidationFlow` _string_ | InvalidationFlow is the slug of the flow used when ending a session. |  | MinLength: 1 <br /> |
| `authenticationFlow` _string_ | AuthenticationFlow is the slug of the flow used to authenticate a user<br />who reaches the application unauthenticated. Leave unset to use<br />authentik's default. |  | Optional: \{\} <br /> |
| `propertyMappings` _string array_ | PropertyMappings are the names of property mappings to attach. |  | Optional: \{\} <br /> |
| `adoptionPolicy` _[AdoptionPolicy](#adoptionpolicy)_ | AdoptionPolicy controls what happens when a provider with this name<br />already exists in authentik. | FailOnConflict | Enum: [FailOnConflict AdoptExisting] <br />Optional: \{\} <br /> |
| `deletionPolicy` _[DeletionPolicy](#deletionpolicy)_ | DeletionPolicy controls what happens to the authentik provider when this<br />resource is deleted. | Delete | Enum: [Delete Orphan] <br />Optional: \{\} <br /> |


#### ProviderStatus



ProviderStatus is the status shared by every provider kind.



_Appears in:_
- [OAuth2ProviderStatus](#oauth2providerstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#condition-v1-meta) array_ | Conditions describe the current state of the resource. |  | Optional: \{\} <br /> |
| `observedGeneration` _integer_ | ObservedGeneration is the .metadata.generation this status reflects. |  | Optional: \{\} <br /> |
| `remoteID` _string_ | RemoteID is authentik's own identifier for the managed object: a numeric<br />primary key for providers and applications, a UUID elsewhere.<br />Once set this is the authoritative handle for the object. Lookups prefer<br />it over the name, so that renaming the object on either side does not<br />cause the operator to lose track of it and create a duplicate. |  | Optional: \{\} <br /> |
| `remoteName` _string_ | RemoteName is the name or slug last observed in authentik. Informational. |  | Optional: \{\} <br /> |
| `adopted` _boolean_ | Adopted records that this resource took over a pre-existing authentik<br />object rather than creating it. |  | Optional: \{\} <br /> |
| `lastSyncedTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.34/#time-v1-meta)_ | LastSyncedTime is when the resource last reconciled successfully. |  | Optional: \{\} <br /> |
| `providerID` _integer_ | ProviderID is authentik's numeric primary key for this provider. It is<br />what an Application or Outpost must reference, so it is surfaced<br />separately from the generic RemoteID string. |  | Optional: \{\} <br /> |


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

<!-- END GENERATED: api -->
