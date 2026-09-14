package authentik

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		wantErr error
	}{
		{
			name:    "empty base URL",
			cfg:     Config{Token: "t"},
			wantErr: ErrValidation,
		},
		{
			name:    "missing scheme",
			cfg:     Config{BaseURL: "authentik.example.com", Token: "t"},
			wantErr: ErrValidation,
		},
		{
			name:    "unsupported scheme",
			cfg:     Config{BaseURL: "ftp://authentik.example.com", Token: "t"},
			wantErr: ErrValidation,
		},
		{
			name:    "empty token",
			cfg:     Config{BaseURL: "https://authentik.example.com", Token: "  "},
			wantErr: ErrUnauthorized,
		},
		{
			name: "invalid CA bundle",
			cfg: Config{
				BaseURL:  "https://authentik.example.com",
				Token:    "t",
				CABundle: []byte("this is not a certificate"),
			},
			wantErr: ErrValidation,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tc.cfg)
			if err == nil {
				t.Fatal("New returned no error")
			}
			if !isSentinel(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewNormalizesBaseURL(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"https://authentik.example.com":           "https://authentik.example.com",
		"https://authentik.example.com/":          "https://authentik.example.com",
		"https://authentik.example.com///":        "https://authentik.example.com",
		"https://authentik.example.com/api/v3":    "https://authentik.example.com",
		"https://authentik.example.com/api/v3/":   "https://authentik.example.com",
		"  https://authentik.example.com/api/v3 ": "https://authentik.example.com",
		"http://authentik.example.com:9000":       "http://authentik.example.com:9000",
		"https://example.com/authentik":           "https://example.com/authentik",
		"https://example.com/authentik/api/v3":    "https://example.com/authentik",
		"https://authentik.example.com?ignored=1": "https://authentik.example.com",
		"https://authentik.example.com#fragment":  "https://authentik.example.com",
	}

	for input, want := range tests {
		client, err := New(Config{BaseURL: input, Token: "token"})
		if err != nil {
			t.Errorf("New(%q): %v", input, err)
			continue
		}
		if got := client.BaseURL(); got != want {
			t.Errorf("New(%q).BaseURL() = %q, want %q", input, got, want)
		}
	}
}

func TestClientSendsAuthAndUserAgent(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	var gotAuth, gotUserAgent string
	fake.handle(pathVersion, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUserAgent = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versionJSON("2026.8.2")))
	})

	client := fake.newClient(t, func(cfg *Config) {
		cfg.Token = "sekret-token"
		cfg.UserAgentSuffix = "v0.1.0"
	})

	if _, err := client.Version(context.Background()); err != nil {
		t.Fatalf("Version: %v", err)
	}
	if gotAuth != "Bearer sekret-token" {
		t.Errorf("Authorization = %q, want a bearer token", gotAuth)
	}
	if !strings.HasPrefix(gotUserAgent, userAgentProduct+"/v0.1.0") {
		t.Errorf("User-Agent = %q, want it to identify the operator and its version", gotUserAgent)
	}
}

func TestConnectionIDIsStableAndNonSecret(t *testing.T) {
	t.Parallel()

	const token = "very-secret-token"

	a, err := New(Config{BaseURL: "https://authentik.example.com", Token: token})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	b, err := New(Config{BaseURL: "https://authentik.example.com/api/v3/", Token: token})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	c, err := New(Config{BaseURL: "https://authentik.example.com", Token: "another-token"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if a.ConnectionID() != b.ConnectionID() {
		t.Error("equivalent base URLs produced different connection IDs")
	}
	if a.ConnectionID() == c.ConnectionID() {
		t.Error("different tokens produced the same connection ID")
	}
	if strings.Contains(a.ConnectionID(), token) {
		t.Errorf("connection ID leaks the API token: %q", a.ConnectionID())
	}
}

func TestClientVersionAndCompatibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		reported  string
		wantLevel SupportLevel
	}{
		// Derived from the bounds rather than hardcoded: pinning a specific
		// series here made this test fail the moment the supported range
		// changed, even though the behaviour under test had not.
		{name: "maximum", reported: MaximumVersion + ".7", wantLevel: SupportSupported},
		{name: "minimum", reported: MinimumVersion + ".0", wantLevel: SupportSupported},
		{name: "too old", reported: "2025.12.1", wantLevel: SupportUnsupported},
		{name: "newer than tested", reported: "2027.10.1", wantLevel: SupportUntested},
		{name: "nonsense", reported: "dev", wantLevel: SupportUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeAuthentik(t)
			fake.respondJSON(pathVersion, http.StatusOK, versionJSON(tc.reported))
			client := fake.newClient(t)

			compat, err := client.Compatibility(context.Background())
			if err != nil {
				t.Fatalf("Compatibility: %v", err)
			}
			if compat.Level != tc.wantLevel {
				t.Errorf("Level = %v, want %v (reported %q)", compat.Level, tc.wantLevel, tc.reported)
			}
			if compat.Raw != tc.reported {
				t.Errorf("Raw = %q, want %q", compat.Raw, tc.reported)
			}
		})
	}
}

func TestClientVersionErrorsAreTyped(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	fake.respondJSON(pathVersion, http.StatusForbidden,
		`{"detail":"You do not have permission to perform this action."}`)
	client := fake.newClient(t)

	if _, err := client.Version(context.Background()); !IsUnauthorized(err) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
	if _, err := client.Compatibility(context.Background()); !IsUnauthorized(err) {
		t.Errorf("err = %v, want ErrUnauthorized", err)
	}
}

func TestClientVersionOfUnreachableInstanceIsTransient(t *testing.T) {
	t.Parallel()

	client, err := New(Config{
		// Reserved TEST-NET-1 address; nothing answers, and the context
		// deadline below keeps the test fast.
		BaseURL: "http://192.0.2.1:9000",
		Token:   "token",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Version(ctx); !IsTransient(err) {
		t.Errorf("err = %v, want a transient error", err)
	}
}

func TestClientAPIExposesGeneratedClient(t *testing.T) {
	t.Parallel()

	fake := newFakeAuthentik(t)
	client := fake.newClient(t)

	if client.API() == nil {
		t.Fatal("API() returned nil")
	}
	if client.API().AdminAPI == nil {
		t.Error("API().AdminAPI is nil")
	}
}
