package authentik

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fixtures -------------------------------------------------------------
//
// The generated models reject any JSON object that omits a required property,
// so these builders emit the full minimal shape authentik would return.

func paginatedJSON(results ...string) string {
	return fmt.Sprintf(`{
		"pagination": {"next": 0, "previous": 0, "count": %d, "current": 1,
		               "total_pages": 1, "start_index": 1, "end_index": %d},
		"results": [%s],
		"autocomplete": {}
	}`, len(results), len(results), strings.Join(results, ","))
}

func flowJSON(pk, slug string) string {
	return fmt.Sprintf(`{
		"pk": %q, "policybindingmodel_ptr_id": %q, "name": "Flow %s", "slug": %q,
		"title": "Flow", "designation": "authentication", "background_url": "/static/bg.jpg",
		"background_themed_urls": null, "stages": [], "policies": [],
		"cache_count": 0, "export_url": "/api/v3/flows/instances/%s/export/"
	}`, pk, pk, slug, slug, slug)
}

func propertyMappingJSON(pk, name string) string {
	return fmt.Sprintf(`{
		"pk": %q, "name": %q, "expression": "return {}", "component": "ak-property-mapping-form",
		"verbose_name": "Property Mapping", "verbose_name_plural": "Property Mappings",
		"meta_model_name": "authentik_core.propertymapping"
	}`, pk, name)
}

func certificateKeyPairJSON(pk, name string) string {
	return fmt.Sprintf(`{
		"pk": %q, "name": %q, "fingerprint_sha256": null, "fingerprint_sha1": null,
		"cert_expiry": null, "cert_subject": null, "private_key_available": true,
		"key_type": null, "certificate_download_url": "/api/v3/crypto/x/",
		"private_key_download_url": "/api/v3/crypto/y/", "managed": null
	}`, pk, name)
}

func providerJSON(pk int32, name string) string {
	return fmt.Sprintf(`{
		"pk": %d, "name": %q, "component": "ak-provider-oauth2-form",
		"assigned_application_slug": null, "assigned_application_name": null,
		"assigned_backchannel_application_slug": null, "assigned_backchannel_application_name": null,
		"verbose_name": "OAuth2/OpenID Provider", "verbose_name_plural": "OAuth2/OpenID Providers",
		"meta_model_name": "authentik_providers_oauth2.oauth2provider"
	}`, pk, name)
}

func serviceConnectionJSON(pk, name string) string {
	return fmt.Sprintf(`{
		"pk": %q, "name": %q, "component": "ak-service-connection-kubernetes-form",
		"verbose_name": "Kubernetes Service-Connection",
		"verbose_name_plural": "Kubernetes Service-Connections",
		"meta_model_name": "authentik_outposts.kubernetesserviceconnection"
	}`, pk, name)
}

func versionJSON(current string) string {
	return fmt.Sprintf(`{
		"version_current": %q, "version_latest": %q, "version_latest_valid": true,
		"build_hash": "", "outdated": false, "outpost_outdated": false
	}`, current, current)
}

// --- fake server ----------------------------------------------------------

// fakeAuthentik is an in-process stand-in for an authentik instance. Tests
// register a handler per API path; every request is counted so cache behaviour
// can be asserted by call volume.
type fakeAuthentik struct {
	server   *httptest.Server
	mu       sync.Mutex
	calls    map[string]int
	queries  map[string][]string
	handlers map[string]http.HandlerFunc
}

func newFakeAuthentik(t *testing.T) *fakeAuthentik {
	t.Helper()

	f := &fakeAuthentik{
		calls:    make(map[string]int),
		queries:  make(map[string][]string),
		handlers: make(map[string]http.HandlerFunc),
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeAuthentik) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls[r.URL.Path]++
	f.queries[r.URL.Path] = append(f.queries[r.URL.Path], r.URL.RawQuery)
	handler, ok := f.handlers[r.URL.Path]
	f.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"Not found."}`))
		return
	}
	handler(w, r)
}

// handle registers a handler for one API path, e.g. "/api/v3/flows/instances/".
func (f *fakeAuthentik) handle(path string, handler http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = handler
}

// respondJSON registers a handler that always answers with status and body.
func (f *fakeAuthentik) respondJSON(path string, status int, body string) {
	f.handle(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func (f *fakeAuthentik) callCount(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[path]
}

func (f *fakeAuthentik) lastQuery(path string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	qs := f.queries[path]
	if len(qs) == 0 {
		return ""
	}
	return qs[len(qs)-1]
}

// newTestClient builds a Client pointed at the fake server.
func (f *fakeAuthentik) newClient(t *testing.T, opts ...func(*Config)) Client {
	t.Helper()

	cfg := Config{
		BaseURL: f.server.URL,
		Token:   "test-token",
		Timeout: 5 * time.Second,
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// API paths the resolvers use, verified against the generated client.
const (
	pathFlows              = "/api/v3/flows/instances/"
	pathPropertyMappings   = "/api/v3/propertymappings/all/"
	pathCertificateKeyPair = "/api/v3/crypto/certificatekeypairs/"
	pathProviders          = "/api/v3/providers/all/"
	pathServiceConnections = "/api/v3/outposts/service_connections/all/"
	pathVersion            = "/api/v3/admin/version/"
)

// --- tests ----------------------------------------------------------------

func TestResolversHitMissAmbiguous(t *testing.T) {
	t.Parallel()

	const (
		uuidA = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001"
		uuidB = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0002"
	)

	tests := []struct {
		name string
		path string
		// body is the list response the fake returns.
		body string
		// resolve runs the resolver under test and returns the resolved value.
		resolve func(context.Context, Client) (string, error)
		wantPK  string
		wantErr error
	}{
		{
			name:    "flow resolves",
			path:    pathFlows,
			body:    paginatedJSON(flowJSON(uuidA, "default-authentication-flow")),
			resolve: resolveFlowFn("default-authentication-flow"),
			wantPK:  uuidA,
		},
		{
			name:    "flow missing",
			path:    pathFlows,
			body:    paginatedJSON(),
			resolve: resolveFlowFn("nope"),
			wantErr: ErrNotFound,
		},
		{
			name:    "flow ambiguous",
			path:    pathFlows,
			body:    paginatedJSON(flowJSON(uuidA, "dup"), flowJSON(uuidB, "dup")),
			resolve: resolveFlowFn("dup"),
			wantErr: ErrAmbiguous,
		},
		{
			name:    "property mapping resolves",
			path:    pathPropertyMappings,
			body:    paginatedJSON(propertyMappingJSON(uuidA, "scope-openid")),
			resolve: resolvePropertyMappingFn("scope-openid"),
			wantPK:  uuidA,
		},
		{
			name:    "property mapping missing",
			path:    pathPropertyMappings,
			body:    paginatedJSON(),
			resolve: resolvePropertyMappingFn("scope-openid"),
			wantErr: ErrNotFound,
		},
		{
			name: "property mapping ambiguous",
			path: pathPropertyMappings,
			body: paginatedJSON(
				propertyMappingJSON(uuidA, "scope-openid"),
				propertyMappingJSON(uuidB, "scope-openid"),
			),
			resolve: resolvePropertyMappingFn("scope-openid"),
			wantErr: ErrAmbiguous,
		},
		{
			name:    "certificate keypair resolves",
			path:    pathCertificateKeyPair,
			body:    paginatedJSON(certificateKeyPairJSON(uuidA, "authentik Self-signed")),
			resolve: resolveCertificateFn("authentik Self-signed"),
			wantPK:  uuidA,
		},
		{
			name:    "certificate keypair missing",
			path:    pathCertificateKeyPair,
			body:    paginatedJSON(),
			resolve: resolveCertificateFn("authentik Self-signed"),
			wantErr: ErrNotFound,
		},
		{
			name: "certificate keypair ambiguous",
			path: pathCertificateKeyPair,
			body: paginatedJSON(
				certificateKeyPairJSON(uuidA, "dup"),
				certificateKeyPairJSON(uuidB, "dup"),
			),
			resolve: resolveCertificateFn("dup"),
			wantErr: ErrAmbiguous,
		},
		{
			name:    "service connection resolves",
			path:    pathServiceConnections,
			body:    paginatedJSON(serviceConnectionJSON(uuidA, "local-kubernetes")),
			resolve: resolveServiceConnectionFn("local-kubernetes"),
			wantPK:  uuidA,
		},
		{
			name:    "service connection missing",
			path:    pathServiceConnections,
			body:    paginatedJSON(),
			resolve: resolveServiceConnectionFn("local-kubernetes"),
			wantErr: ErrNotFound,
		},
		{
			name: "service connection ambiguous",
			path: pathServiceConnections,
			body: paginatedJSON(
				serviceConnectionJSON(uuidA, "dup"),
				serviceConnectionJSON(uuidB, "dup"),
			),
			resolve: resolveServiceConnectionFn("dup"),
			wantErr: ErrAmbiguous,
		},
		{
			name:    "provider resolves",
			path:    pathProviders,
			body:    paginatedJSON(providerJSON(42, "my-provider")),
			resolve: resolveProviderFn("my-provider"),
			wantPK:  "42",
		},
		{
			name:    "provider missing",
			path:    pathProviders,
			body:    paginatedJSON(),
			resolve: resolveProviderFn("my-provider"),
			wantErr: ErrNotFound,
		},
		{
			name:    "provider ambiguous",
			path:    pathProviders,
			body:    paginatedJSON(providerJSON(42, "dup"), providerJSON(43, "dup")),
			resolve: resolveProviderFn("dup"),
			wantErr: ErrAmbiguous,
		},
		{
			// The providers endpoint has no exact-name filter, only a fuzzy
			// search, so near-misses must be discarded client-side.
			name:    "provider search near-miss is not a match",
			path:    pathProviders,
			body:    paginatedJSON(providerJSON(42, "my-provider-staging")),
			resolve: resolveProviderFn("my-provider"),
			wantErr: ErrNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeAuthentik(t)
			fake.respondJSON(tc.path, http.StatusOK, tc.body)
			client := fake.newClient(t)

			got, err := tc.resolve(context.Background(), client)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("resolve = %q, want error %v", got, tc.wantErr)
				}
				if !isSentinel(err, tc.wantErr) {
					t.Fatalf("resolve error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve returned error: %v", err)
			}
			if got != tc.wantPK {
				t.Errorf("resolve = %q, want %q", got, tc.wantPK)
			}
		})
	}
}

func resolveFlowFn(slug string) func(context.Context, Client) (string, error) {
	return func(ctx context.Context, c Client) (string, error) { return c.ResolveFlow(ctx, slug) }
}

func resolvePropertyMappingFn(name string) func(context.Context, Client) (string, error) {
	return func(ctx context.Context, c Client) (string, error) { return c.ResolvePropertyMapping(ctx, name) }
}

func resolveCertificateFn(name string) func(context.Context, Client) (string, error) {
	return func(ctx context.Context, c Client) (string, error) {
		return c.ResolveCertificateKeyPair(ctx, name)
	}
}

func resolveServiceConnectionFn(name string) func(context.Context, Client) (string, error) {
	return func(ctx context.Context, c Client) (string, error) {
		return c.ResolveServiceConnection(ctx, name)
	}
}

func resolveProviderFn(name string) func(context.Context, Client) (string, error) {
	return func(ctx context.Context, c Client) (string, error) {
		pk, err := c.ResolveProvider(ctx, name)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%d", pk), nil
	}
}

func isSentinel(err, want error) bool {
	switch want {
	case ErrNotFound:
		return IsNotFound(err)
	case ErrAmbiguous:
		return IsAmbiguous(err)
	case ErrValidation:
		return IsValidation(err)
	case ErrUnauthorized:
		return IsUnauthorized(err)
	case ErrConflict:
		return IsConflict(err)
	case ErrTransient:
		return IsTransient(err)
	default:
		return false
	}
}

func TestResolveUsesExactServerSideFilter(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	fake.respondJSON(pathFlows, http.StatusOK,
		paginatedJSON(flowJSON("1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001", "my-flow")))
	client := fake.newClient(t)

	if _, err := client.ResolveFlow(context.Background(), "my-flow"); err != nil {
		t.Fatalf("ResolveFlow: %v", err)
	}
	if q := fake.lastQuery(pathFlows); !strings.Contains(q, "slug=my-flow") {
		t.Errorf("query = %q, want it to filter on slug", q)
	}
}

func TestResolvePassesThroughUUIDsWithoutCalling(t *testing.T) {
	t.Parallel()

	const (
		hyphenated = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001"
		compact    = "1e2ff1a00e574c4e8f9f2a3f5f5c0001"
		uppercase  = "1E2FF1A0-0E57-4C4E-8F9F-2A3F5F5C0001"
	)

	fake := newFakeAuthentik(t)
	client := fake.newClient(t)
	ctx := context.Background()

	for _, value := range []string{hyphenated, compact, uppercase} {
		got, err := client.ResolveFlow(ctx, value)
		if err != nil {
			t.Fatalf("ResolveFlow(%q): %v", value, err)
		}
		if got != value {
			t.Errorf("ResolveFlow(%q) = %q, want it passed through", value, got)
		}
	}
	if n := fake.callCount(pathFlows); n != 0 {
		t.Errorf("UUID passthrough made %d API calls, want 0", n)
	}

	// Providers are keyed by an integer, so an integer passes through instead.
	pk, err := client.ResolveProvider(ctx, "77")
	if err != nil {
		t.Fatalf("ResolveProvider(77): %v", err)
	}
	if pk != 77 {
		t.Errorf("ResolveProvider(77) = %d, want 77", pk)
	}
	if n := fake.callCount(pathProviders); n != 0 {
		t.Errorf("integer passthrough made %d API calls, want 0", n)
	}
}

func TestIsUUID(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001":     true,
		"1E2FF1A0-0E57-4C4E-8F9F-2A3F5F5C0001":     true,
		"1e2ff1a00e574c4e8f9f2a3f5f5c0001":         true,
		"  1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001  ": true,
		"default-authentication-flow":              false,
		"":                                         false,
		"1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c000":      false,
		"1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c00011":    false,
		"zzzzzzzz-0e57-4c4e-8f9f-2a3f5f5c0001":     false,
	}
	for value, want := range tests {
		if got := IsUUID(value); got != want {
			t.Errorf("IsUUID(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestResolveCachesSuccessAndInvalidatesOnMiss(t *testing.T) {
	t.Parallel()

	const uuid = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001"

	var present bool
	fake := newFakeAuthentik(t)
	fake.handle(pathFlows, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if present {
			_, _ = w.Write([]byte(paginatedJSON(flowJSON(uuid, "late-flow"))))
			return
		}
		_, _ = w.Write([]byte(paginatedJSON()))
	})

	client := fake.newClient(t, func(cfg *Config) { cfg.CacheTTL = time.Hour })
	ctx := context.Background()

	// A miss is never cached: the flow may show up at any moment.
	for i := range 3 {
		if _, err := client.ResolveFlow(ctx, "late-flow"); !IsNotFound(err) {
			t.Fatalf("attempt %d: err = %v, want ErrNotFound", i, err)
		}
	}
	if n := fake.callCount(pathFlows); n != 3 {
		t.Errorf("misses made %d calls, want 3 (a miss must not be cached)", n)
	}

	// Once the flow exists it is picked up immediately, then served from cache.
	present = true
	got, err := client.ResolveFlow(ctx, "late-flow")
	if err != nil {
		t.Fatalf("ResolveFlow after creation: %v", err)
	}
	if got != uuid {
		t.Errorf("ResolveFlow = %q, want %q", got, uuid)
	}

	for range 5 {
		if _, err := client.ResolveFlow(ctx, "late-flow"); err != nil {
			t.Fatalf("cached ResolveFlow: %v", err)
		}
	}
	if n := fake.callCount(pathFlows); n != 4 {
		t.Errorf("total calls = %d, want 4 (3 misses + 1 hit, the rest cached)", n)
	}

	// InvalidateCache forces the next lookup back to the API.
	client.InvalidateCache()
	if _, err := client.ResolveFlow(ctx, "late-flow"); err != nil {
		t.Fatalf("ResolveFlow after invalidation: %v", err)
	}
	if n := fake.callCount(pathFlows); n != 5 {
		t.Errorf("total calls = %d, want 5 after cache invalidation", n)
	}
}

// TestResolveCacheIsolatesConnections points two clients with distinct tokens
// at one shared cache and checks neither reads the other's entries.
func TestResolveCacheIsolatesConnections(t *testing.T) {
	t.Parallel()

	const (
		uuidA = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c000a"
		uuidB = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c000b"
	)

	fakeA := newFakeAuthentik(t)
	fakeA.respondJSON(pathFlows, http.StatusOK, paginatedJSON(flowJSON(uuidA, "shared-slug")))
	fakeB := newFakeAuthentik(t)
	fakeB.respondJSON(pathFlows, http.StatusOK, paginatedJSON(flowJSON(uuidB, "shared-slug")))

	shared := NewRefCache(time.Hour)
	clientA := fakeA.newClient(t, func(cfg *Config) { cfg.Cache = shared; cfg.Token = "token-a" })
	clientB := fakeB.newClient(t, func(cfg *Config) { cfg.Cache = shared; cfg.Token = "token-b" })

	ctx := context.Background()
	gotA, err := clientA.ResolveFlow(ctx, "shared-slug")
	if err != nil {
		t.Fatalf("clientA.ResolveFlow: %v", err)
	}
	gotB, err := clientB.ResolveFlow(ctx, "shared-slug")
	if err != nil {
		t.Fatalf("clientB.ResolveFlow: %v", err)
	}

	if gotA != uuidA {
		t.Errorf("clientA resolved %q, want %q", gotA, uuidA)
	}
	if gotB != uuidB {
		t.Errorf("clientB resolved %q, want %q (it read the other instance's cache entry)", gotB, uuidB)
	}
	if clientA.ConnectionID() == clientB.ConnectionID() {
		t.Error("two connections share a ConnectionID")
	}

	// Invalidating one connection must not disturb the other.
	clientA.InvalidateCache()
	if _, ok := shared.Get(clientB.ConnectionID(), KindFlow, "shared-slug"); !ok {
		t.Error("invalidating connection A dropped connection B's entry")
	}
}

func TestResolveCacheExpiresBetweenReconciles(t *testing.T) {
	t.Parallel()

	const uuid = "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001"

	fake := newFakeAuthentik(t)
	fake.respondJSON(pathFlows, http.StatusOK, paginatedJSON(flowJSON(uuid, "a-flow")))

	cache, advance := newTestCache(t, 30*time.Second)
	client := fake.newClient(t, func(cfg *Config) { cfg.Cache = cache })
	ctx := context.Background()

	if _, err := client.ResolveFlow(ctx, "a-flow"); err != nil {
		t.Fatalf("ResolveFlow: %v", err)
	}
	if _, err := client.ResolveFlow(ctx, "a-flow"); err != nil {
		t.Fatalf("ResolveFlow: %v", err)
	}
	if n := fake.callCount(pathFlows); n != 1 {
		t.Fatalf("calls = %d, want 1 (second lookup should be cached)", n)
	}

	advance(31 * time.Second)
	if _, err := client.ResolveFlow(ctx, "a-flow"); err != nil {
		t.Fatalf("ResolveFlow after expiry: %v", err)
	}
	if n := fake.callCount(pathFlows); n != 2 {
		t.Errorf("calls = %d, want 2 (cache should have expired)", n)
	}
}

func TestResolveMapsAPIErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{"detail":"bad token"}`, wantErr: ErrUnauthorized},
		{name: "forbidden", status: http.StatusForbidden, body: `{"detail":"nope"}`, wantErr: ErrUnauthorized},
		{name: "server error", status: http.StatusInternalServerError, body: `{"detail":"boom"}`, wantErr: ErrTransient},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"detail":"slow down"}`, wantErr: ErrTransient},
		{name: "bad request", status: http.StatusBadRequest, body: `{"slug":["invalid"]}`, wantErr: ErrValidation},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeAuthentik(t)
			fake.respondJSON(pathFlows, tc.status, tc.body)
			client := fake.newClient(t)

			_, err := client.ResolveFlow(context.Background(), "some-flow")
			if err == nil {
				t.Fatal("ResolveFlow returned no error")
			}
			if !isSentinel(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestResolvePropertyMappingsPreservesOrder(t *testing.T) {
	t.Parallel()

	uuids := map[string]string{
		"scope-openid":  "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0001",
		"scope-email":   "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0002",
		"scope-profile": "1e2ff1a0-0e57-4c4e-8f9f-2a3f5f5c0003",
	}

	fake := newFakeAuthentik(t)
	fake.handle(pathPropertyMappings, func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		w.Header().Set("Content-Type", "application/json")
		pk, ok := uuids[name]
		if !ok {
			_, _ = w.Write([]byte(paginatedJSON()))
			return
		}
		_, _ = w.Write([]byte(paginatedJSON(propertyMappingJSON(pk, name))))
	})
	client := fake.newClient(t)

	names := []string{"scope-profile", "scope-openid", "scope-email"}
	got, err := client.ResolvePropertyMappings(context.Background(), names)
	if err != nil {
		t.Fatalf("ResolvePropertyMappings: %v", err)
	}
	for i, name := range names {
		if got[i] != uuids[name] {
			t.Errorf("index %d = %q, want %q", i, got[i], uuids[name])
		}
	}

	if _, err := client.ResolvePropertyMappings(context.Background(),
		[]string{"scope-openid", "missing"}); !IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound for the missing name", err)
	}
}

func TestResolveEmptyNameIsNotFound(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	client := fake.newClient(t)
	ctx := context.Background()

	if _, err := client.ResolveFlow(ctx, "   "); !IsNotFound(err) {
		t.Errorf("ResolveFlow(blank) = %v, want ErrNotFound", err)
	}
	if _, err := client.ResolveProvider(ctx, ""); !IsNotFound(err) {
		t.Errorf("ResolveProvider(blank) = %v, want ErrNotFound", err)
	}
	if fake.callCount(pathFlows)+fake.callCount(pathProviders) != 0 {
		t.Error("a blank reference reached the API")
	}
}

func TestResolveHonoursContextCancellation(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	fake.respondJSON(pathFlows, http.StatusOK, paginatedJSON())
	client := fake.newClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.ResolveFlow(ctx, "a-flow")
	if !IsTransient(err) {
		t.Errorf("err = %v, want a transient error for a cancelled context", err)
	}
}
