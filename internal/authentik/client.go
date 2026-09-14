package authentik

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	api "goauthentik.io/api/v3"
)

// Default tuning for the HTTP transport. The values are deliberately modest:
// every call happens inside a reconcile, which has its own deadline, and a
// hung connection must never pin a worker.
const (
	// DefaultTimeout bounds a single API call end to end.
	DefaultTimeout = 30 * time.Second
	// DefaultDialTimeout bounds establishing a TCP connection.
	DefaultDialTimeout = 10 * time.Second
	// DefaultTLSHandshakeTimeout bounds the TLS handshake.
	DefaultTLSHandshakeTimeout = 10 * time.Second
	// DefaultResponseHeaderTimeout bounds the wait for response headers.
	DefaultResponseHeaderTimeout = 20 * time.Second
	// DefaultIdleConnTimeout bounds how long a pooled connection stays open.
	DefaultIdleConnTimeout = 90 * time.Second
	// defaultMaxIdleConnsPerHost keeps connections warm for the handful of
	// authentik instances a single operator talks to.
	defaultMaxIdleConnsPerHost = 8
)

// apiBasePath is the prefix every authentik REST endpoint lives under.
const apiBasePath = "/api/v3"

// userAgentProduct identifies this operator to authentik. Instances show the
// User-Agent in their event log, which is how an administrator tells operator
// traffic apart from a human in the web UI.
const userAgentProduct = "authentik-operator"

// Client is the surface the operator's controllers use to talk to one
// authentik instance. It is an interface so controllers can be unit-tested
// against a fake without an authentik server.
//
// It intentionally exposes only reference resolution and version negotiation —
// the cross-cutting concerns every controller needs — rather than mirroring the
// authentik API. Resource-specific CRUD is reached through API, which hands
// back the generated client, so adding a new resource kind does not mean
// widening this interface.
type Client interface {
	// BaseURL returns the authentik base URL this client talks to, without the
	// /api/v3 suffix.
	BaseURL() string

	// ConnectionID returns a stable, non-secret identifier for this
	// (base URL, token) pair. It is the cache partition key and is safe to log.
	ConnectionID() string

	// Version returns the authentik version the instance is running.
	Version(ctx context.Context) (Version, error)

	// Compatibility returns the running version together with a verdict on
	// whether this operator supports it.
	Compatibility(ctx context.Context) (Compatibility, error)

	// ResolveFlow resolves a flow slug to its UUID. A value that is already a
	// UUID is returned unchanged.
	ResolveFlow(ctx context.Context, slug string) (string, error)

	// ResolvePropertyMapping resolves a property mapping name to its UUID.
	ResolvePropertyMapping(ctx context.Context, name string) (string, error)

	// ResolvePropertyMappings resolves several property mapping names,
	// preserving order. It fails on the first name that does not resolve.
	ResolvePropertyMappings(ctx context.Context, names []string) ([]string, error)

	// ResolveCertificateKeyPair resolves a certificate-key pair name to its UUID.
	ResolveCertificateKeyPair(ctx context.Context, name string) (string, error)

	// ResolveProvider resolves a provider name to its integer primary key. A
	// value that is already an integer is returned unchanged.
	ResolveProvider(ctx context.Context, name string) (int32, error)

	// ResolveServiceConnection resolves an outpost service connection name to
	// its UUID.
	ResolveServiceConnection(ctx context.Context, name string) (string, error)

	// InvalidateCache drops every cached reference for this connection. Call it
	// when a reconcile observes state that contradicts a cached lookup.
	InvalidateCache()

	// API exposes the underlying generated client for resource-specific calls.
	// Errors it returns are the generated client's own and must be passed
	// through MapResponseError before they reach a caller or a log.
	API() *api.APIClient
}

// Config describes one authentik connection.
type Config struct {
	// BaseURL is the root URL of the authentik instance, for example
	// https://authentik.example.com. A trailing /api/v3 is accepted and
	// trimmed. Required.
	BaseURL string

	// Token is an authentik API token, sent as a bearer token. Required.
	Token string

	// CABundle is an optional PEM bundle used to verify the instance's
	// certificate. When set it replaces the system roots.
	CABundle []byte

	// InsecureSkipVerify disables TLS certificate verification. It exists for
	// development against self-signed instances and must not be used otherwise.
	InsecureSkipVerify bool

	// Timeout bounds a single API call. Defaults to DefaultTimeout.
	Timeout time.Duration

	// CacheTTL is the lifetime of cached reference lookups. Ignored when Cache
	// is supplied. Defaults to DefaultCacheTTL.
	CacheTTL time.Duration

	// Cache is an optional shared reference cache. Supplying one lets many
	// clients share a cache; entries stay partitioned by connection either way.
	Cache *RefCache

	// UserAgentSuffix is appended to the operator's User-Agent, typically the
	// operator version.
	UserAgentSuffix string

	// Transport overrides the HTTP transport. It is meant for tests; leaving it
	// nil builds a transport from the TLS and timeout settings above.
	Transport http.RoundTripper
}

// client is the production implementation of Client.
type client struct {
	api      *api.APIClient
	baseURL  string
	connID   string
	cache    *RefCache
	pageSize int32
}

// compile-time assertion that the implementation satisfies the interface.
var _ Client = (*client)(nil)

// New builds a Client for one authentik instance.
func New(cfg Config) (Client, error) {
	baseURL, err := normalizeBaseURL(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, fmt.Errorf("%w: authentik API token is empty", ErrUnauthorized)
	}

	transport := cfg.Transport
	if transport == nil {
		transport, err = newTransport(cfg)
		if err != nil {
			return nil, err
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	userAgent := buildUserAgent(cfg.UserAgentSuffix)
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &authTransport{
			next:      transport,
			token:     cfg.Token,
			userAgent: userAgent,
		},
	}

	apiCfg := api.NewConfiguration()
	apiCfg.UserAgent = userAgent
	apiCfg.HTTPClient = httpClient
	apiCfg.Servers = api.ServerConfigurations{{
		URL:         baseURL + apiBasePath,
		Description: "authentik instance managed by " + userAgentProduct,
	}}

	cache := cfg.Cache
	if cache == nil {
		cache = NewRefCache(cfg.CacheTTL)
	}

	return &client{
		api:      api.NewAPIClient(apiCfg),
		baseURL:  baseURL,
		connID:   connectionID(baseURL, cfg.Token),
		cache:    cache,
		pageSize: defaultPageSize,
	}, nil
}

// BaseURL implements Client.
func (c *client) BaseURL() string { return c.baseURL }

// ConnectionID implements Client.
func (c *client) ConnectionID() string { return c.connID }

// API implements Client.
func (c *client) API() *api.APIClient { return c.api }

// InvalidateCache implements Client.
func (c *client) InvalidateCache() { c.cache.InvalidateConnection(c.connID) }

// MapResponseError converts the (*http.Response, error) pair returned by a
// generated API method into one of this package's typed errors. Callers that
// reach past Client.API must funnel every failure through it, both to get
// errors.Is-able results and to keep the generated client's error value — which
// carries the raw response body — from escaping.
//
// op is a short static description such as "create provider"; it must not
// contain user data.
func MapResponseError(op string, resp *http.Response, err error) error {
	return mapResponse(op, resp, err)
}

// normalizeBaseURL validates the configured URL and strips any trailing slash
// or /api/v3 suffix, so both forms an administrator might paste work.
func normalizeBaseURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: authentik base URL is empty", ErrValidation)
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: authentik base URL is not a valid URL", ErrValidation)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: authentik base URL must use http or https, got %q", ErrValidation, parsed.Scheme)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("%w: authentik base URL has no host", ErrValidation)
	}

	parsed.Path = strings.TrimSuffix(strings.TrimRight(parsed.Path, "/"), apiBasePath)
	parsed.RawQuery = ""
	parsed.Fragment = ""

	// Drop any userinfo. authentik authenticates with a bearer token, so
	// credentials in the URL are never meaningful - but url.URL.String()
	// renders a password in full (unlike Redacted), so a dial failure would
	// otherwise print it into a condition message.
	parsed.User = nil
	return strings.TrimRight(parsed.String(), "/"), nil
}

// newTransport builds an http.Transport with bounded timeouts and the
// configured TLS trust settings.
func newTransport(cfg Config) (http.RoundTripper, error) {
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		//nolint:gosec // G402: opt-in, documented as development-only.
		InsecureSkipVerify: cfg.InsecureSkipVerify,
	}
	if len(cfg.CABundle) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CABundle) {
			return nil, fmt.Errorf("%w: CA bundle contains no valid PEM certificate", ErrValidation)
		}
		tlsCfg.RootCAs = pool
	}

	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   DefaultDialTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSClientConfig:       tlsCfg,
		TLSHandshakeTimeout:   DefaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   defaultMaxIdleConnsPerHost,
	}, nil
}

// authTransport attaches the bearer token and User-Agent to every request. The
// token lives here rather than in a request context so it is set exactly once
// and never travels through call sites that might log their context.
type authTransport struct {
	next      http.RoundTripper
	token     string
	userAgent string
}

// RoundTrip implements http.RoundTripper.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Per the RoundTripper contract the request must not be modified in place.
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+t.token)
	clone.Header.Set("User-Agent", t.userAgent)
	if clone.Header.Get("Accept") == "" {
		clone.Header.Set("Accept", "application/json")
	}

	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(clone)
}

// buildUserAgent renders the User-Agent sent on every request.
func buildUserAgent(suffix string) string {
	ua := userAgentProduct
	if s := strings.TrimSpace(suffix); s != "" {
		ua += "/" + s
	}
	return ua + " (+https://github.com/goauthentik/authentik)"
}

// connectionID derives a stable, non-reversible identifier for a connection.
// The token is hashed in: rotating a token, or pointing two credentials at the
// same host, must not let one connection read another's cached references.
func connectionID(baseURL, token string) string {
	sum := sha256.Sum256([]byte(baseURL + "\x00" + token))
	return baseURL + "#" + hex.EncodeToString(sum[:8])
}
