package authentik

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestMapErrorStatusClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		body        string
		wantKind    error
		wantNoKind  bool
		wantRetry   bool
		wantDetail  string
		wantCode    string
		wantMessage string
	}{
		{
			name:     "bad request is validation",
			status:   http.StatusBadRequest,
			body:     `{"name":["This field is required."]}`,
			wantKind: ErrValidation,
		},
		{
			name:     "unprocessable entity is validation",
			status:   http.StatusUnprocessableEntity,
			body:     `{"non_field_errors":["Invalid combination."]}`,
			wantKind: ErrValidation,
		},
		{
			name:       "unauthorized",
			status:     http.StatusUnauthorized,
			body:       `{"detail":"Authentication credentials were not provided."}`,
			wantKind:   ErrUnauthorized,
			wantDetail: "Authentication credentials were not provided.",
		},
		{
			name:     "forbidden maps to unauthorized",
			status:   http.StatusForbidden,
			body:     `{"detail":"You do not have permission.","code":"permission_denied"}`,
			wantKind: ErrUnauthorized,
			wantCode: "permission_denied",
		},
		{
			name:     "not found",
			status:   http.StatusNotFound,
			body:     `{"detail":"Not found."}`,
			wantKind: ErrNotFound,
		},
		{
			name:     "conflict",
			status:   http.StatusConflict,
			body:     `{"detail":"already exists"}`,
			wantKind: ErrConflict,
		},
		{
			name:      "request timeout is transient",
			status:    http.StatusRequestTimeout,
			wantKind:  ErrTransient,
			wantRetry: true,
		},
		{
			name:      "too many requests is transient",
			status:    http.StatusTooManyRequests,
			wantKind:  ErrTransient,
			wantRetry: true,
		},
		{
			name:      "internal server error is transient",
			status:    http.StatusInternalServerError,
			body:      `{"detail":"boom"}`,
			wantKind:  ErrTransient,
			wantRetry: true,
		},
		{
			name:      "bad gateway is transient",
			status:    http.StatusBadGateway,
			wantKind:  ErrTransient,
			wantRetry: true,
		},
		{
			name:      "service unavailable is transient",
			status:    http.StatusServiceUnavailable,
			wantKind:  ErrTransient,
			wantRetry: true,
		},
		{
			name:       "unclassified 4xx carries no sentinel",
			status:     http.StatusTeapot,
			wantNoKind: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := MapError(tc.status, []byte(tc.body))
			if err == nil {
				t.Fatalf("MapError(%d) = nil, want error", tc.status)
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("MapError(%d) returned %T, want *APIError", tc.status, err)
			}
			if apiErr.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tc.status)
			}
			if tc.wantNoKind {
				if apiErr.Kind != nil {
					t.Errorf("Kind = %v, want nil", apiErr.Kind)
				}
			} else if !errors.Is(err, tc.wantKind) {
				t.Errorf("errors.Is(err, %v) = false; err = %v", tc.wantKind, err)
			}
			if got := apiErr.Retryable(); got != tc.wantRetry {
				t.Errorf("Retryable() = %v, want %v", got, tc.wantRetry)
			}
			if tc.wantDetail != "" && apiErr.Detail != tc.wantDetail {
				t.Errorf("Detail = %q, want %q", apiErr.Detail, tc.wantDetail)
			}
			if tc.wantCode != "" && apiErr.Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", apiErr.Code, tc.wantCode)
			}
			if apiErr.Error() == "" {
				t.Error("Error() returned an empty string")
			}
		})
	}
}

func TestMapErrorSuccessStatusIsNil(t *testing.T) {
	t.Parallel()

	for _, status := range []int{200, 201, 204, 299} {
		if err := MapError(status, []byte(`{"detail":"ignored"}`)); err != nil {
			t.Errorf("MapError(%d) = %v, want nil", status, err)
		}
	}
}

func TestMapErrorCarriesFieldValidationMessages(t *testing.T) {
	t.Parallel()

	body := `{
		"name": ["This field is required.", "Enter a valid name."],
		"authorization_flow": ["Object does not exist."],
		"non_field_errors": ["A provider with this name already exists."]
	}`

	err := MapError(http.StatusBadRequest, []byte(body))
	if !IsValidation(err) {
		t.Fatalf("IsValidation(err) = false; err = %v", err)
	}

	fields, ok := FieldErrors(err)
	if !ok {
		t.Fatal("FieldErrors(err) reported no fields")
	}
	for _, field := range []string{"name", "authorization_flow", "non_field_errors"} {
		if len(fields[field]) == 0 {
			t.Errorf("field %q carried no messages, got %#v", field, fields)
		}
	}
	if got, want := len(fields["name"]), 2; got != want {
		t.Errorf("len(fields[name]) = %d, want %d", got, want)
	}

	// The messages must survive into the rendered error: this is how a user
	// debugs a bad spec from a status condition.
	msg := err.Error()
	for _, want := range []string{"name", "This field is required.", "Object does not exist."} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, want it to contain %q", msg, want)
		}
	}
}

func TestFieldErrorsReturnsCopy(t *testing.T) {
	t.Parallel()

	err := MapError(http.StatusBadRequest, []byte(`{"name":["required"]}`))
	fields, ok := FieldErrors(err)
	if !ok {
		t.Fatal("FieldErrors reported no fields")
	}
	fields["name"][0] = "mutated"
	delete(fields, "name")

	again, _ := FieldErrors(err)
	if got := again["name"]; len(got) != 1 || got[0] != "required" {
		t.Errorf("mutating the returned map changed the error: %#v", again)
	}
}

// TestErrorNeverLeaksSecrets is the guard for the rule that no secret sent to
// authentik may travel back out inside an error. It feeds the mapper the worst
// case: an error response that echoes the submitted payload verbatim.
func TestErrorNeverLeaksSecrets(t *testing.T) {
	t.Parallel()

	const secret = "SUPER-SECRET-CLIENT-VALUE-9f2a"

	bodies := []struct {
		name string
		body string
	}{
		{
			name: "echoed request payload alongside field errors",
			body: `{"client_secret":"` + secret + `","name":["This field is required."]}`,
		},
		{
			name: "secret inside a field error message",
			body: `{"client_secret":["Value ` + secret + ` is not valid."]}`,
		},
		{
			name: "secret under a nested object",
			body: `{"provider":{"client_secret":"` + secret + `"}}`,
		},
		{
			name: "secret in a non-JSON body",
			body: `Internal Server Error while saving client_secret=` + secret,
		},
		{
			name: "secret under an alternate credential field name",
			body: `{"bind_password":["rejected value ` + secret + `"],"token":["bad ` + secret + `"]}`,
		},
	}

	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, status := range []int{http.StatusBadRequest, http.StatusInternalServerError, http.StatusConflict} {
				err := MapError(status, []byte(tc.body))
				if err == nil {
					t.Fatalf("MapError(%d) = nil", status)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("status %d: error string leaked the secret: %q", status, err.Error())
				}
				fields, _ := FieldErrors(err)
				for name, msgs := range fields {
					for _, msg := range msgs {
						if strings.Contains(msg, secret) {
							t.Errorf("status %d: field %q leaked the secret: %q", status, name, msg)
						}
					}
				}
			}
		})
	}
}

func TestMapResponseDropsGeneratedClientError(t *testing.T) {
	t.Parallel()

	const secret = "REQUEST-BODY-SECRET-0001"

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"name":["This field is required."]}`)),
	}
	// Stand in for the generated client's error, which renders the response
	// status and can carry the raw body on an accessor.
	generated := errors.New("400 Bad Request: posted {\"client_secret\":\"" + secret + "\"}")

	err := MapResponseError("create provider", resp, generated)
	if !IsValidation(err) {
		t.Fatalf("IsValidation(err) = false; err = %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("mapped error leaked the generated error's payload: %q", err.Error())
	}
	if errors.Is(err, generated) {
		t.Error("mapped error wraps the generated error; its Body() accessor stays reachable")
	}
	if !strings.Contains(err.Error(), "create provider") {
		t.Errorf("Error() = %q, want it to name the operation", err.Error())
	}
}

func TestMapResponseWithoutResponseIsTransient(t *testing.T) {
	t.Parallel()

	err := MapResponseError("list flows", nil, errors.New("dial tcp: connection refused"))
	if !IsTransient(err) {
		t.Fatalf("IsTransient(err) = false; err = %v", err)
	}
	if MapResponseError("list flows", nil, nil) != nil {
		t.Error("MapResponseError with a nil error returned non-nil")
	}
}

func TestMapResponseOnUndecodableSuccessIsTransient(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("not json")),
	}
	err := MapResponseError("list flows", resp, errors.New("invalid character 'o'"))
	if !IsTransient(err) {
		t.Fatalf("IsTransient(err) = false; err = %v", err)
	}
}

func TestErrorHelpersAreMutuallyExclusive(t *testing.T) {
	t.Parallel()

	cases := map[error]func(error) bool{
		ErrNotFound:     IsNotFound,
		ErrConflict:     IsConflict,
		ErrUnauthorized: IsUnauthorized,
		ErrValidation:   IsValidation,
		ErrTransient:    IsTransient,
		ErrAmbiguous:    IsAmbiguous,
	}

	for sentinel, match := range cases {
		wrapped := &APIError{Kind: sentinel, Op: "test"}
		if !match(wrapped) {
			t.Errorf("helper for %v did not match its own sentinel", sentinel)
		}
		for other, otherMatch := range cases {
			if other == sentinel {
				continue
			}
			if otherMatch(wrapped) {
				t.Errorf("helper for %v matched an error wrapping %v", other, sentinel)
			}
		}
	}
}

func TestMapErrorTruncatesOversizedMessages(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", maxErrorMessageLength*3)
	err := MapError(http.StatusBadRequest, []byte(`{"detail":"`+long+`"}`))

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("got %T, want *APIError", err)
	}
	if len(apiErr.Detail) > maxErrorMessageLength+len("…") {
		t.Errorf("Detail length = %d, want it truncated to %d", len(apiErr.Detail), maxErrorMessageLength)
	}
}

// TestErrorRedactsOperatorSubmittedCredentialFields covers a gap a security
// audit found: the deny-list held authentik's own credential field names but
// not the ones this operator submits. A kubeconfig carries client-key-data and
// bearer tokens, and an outpost config is free-form, so either could reach a
// condition message - readable by anyone with get on the custom resource.
func TestErrorRedactsOperatorSubmittedCredentialFields(t *testing.T) {
	t.Parallel()

	secret := "s3cr3t-value-that-must-not-escape"

	for _, field := range []string{"kubeconfig", "config"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			body := []byte(`{"` + field + `": ["rejected: ` + secret + `"]}`)
			err := MapError(http.StatusBadRequest, body)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), secret) {
				t.Errorf("field %q leaked its value: %s", field, err.Error())
			}
		})
	}
}

// TestErrorKeepsMessagesForNonCredentialFields is the other half of the same
// decision, and the reason the deny-list stayed narrow.
//
// Substrings like url, key, cert and auth match acs_url, signing_key,
// certificate and authorization_flow - fields whose validation messages are
// exactly what a user needs to fix a bad spec. Blanket-redacting them by name
// would trade a real diagnostic for an imagined leak; credential-shaped values
// inside those messages are caught by shape instead.
func TestErrorKeepsMessagesForNonCredentialFields(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"acs_url":            "Enter a valid URL.",
		"signing_key":        "Object does not exist.",
		"certificate":        "Object does not exist.",
		"authorization_flow": "Object does not exist.",
	}

	for field, message := range cases {
		t.Run(field, func(t *testing.T) {
			t.Parallel()

			body := []byte(`{"` + field + `": ["` + message + `"]}`)
			err := MapError(http.StatusBadRequest, body)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), message) {
				t.Errorf("field %q lost its diagnostic message: %s", field, err.Error())
			}
		})
	}
}

// A credential quoted inside a message on an innocuous field is still scrubbed,
// which is what lets the deny-list stay narrow.
func TestErrorScrubsCredentialShapedValuesInAnyField(t *testing.T) {
	t.Parallel()

	body := []byte(`{"acs_url": ["rejected client_secret=hunter2-must-not-appear"]}`)
	err := MapError(http.StatusBadRequest, body)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "hunter2-must-not-appear") {
		t.Errorf("a credential-shaped value escaped through a non-credential field: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "acs_url") {
		t.Errorf("the field name should survive so the message stays useful: %s", err.Error())
	}
}

// TestErrorRedactsCredentialsInDetail covers the second half of that gap: the
// top-level "detail" string was passed through with no scrubbing at all, so a
// credential quoted in prose rather than attached to a field name escaped.
func TestErrorRedactsCredentialsInDetail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		detail string
		secret string
	}{
		{
			name:   "client secret in prose",
			detail: "Validation failed: client_secret=hunter2-should-not-appear",
			secret: "hunter2-should-not-appear",
		},
		{
			name:   "bearer token",
			detail: "upstream rejected authorization: Bearer abcdef-leaky-token",
			secret: "abcdef-leaky-token",
		},
		{
			name:   "kubeconfig key material",
			detail: "invalid kubeconfig: client-key-data: LS0tLS1CRUdJTlBSSVZBVEU",
			secret: "LS0tLS1CRUdJTlBSSVZBVEU",
		},
		{
			name:   "password assignment",
			detail: "could not bind, password = correct-horse-battery",
			secret: "correct-horse-battery",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			body, marshalErr := json.Marshal(map[string]string{"detail": tc.detail})
			if marshalErr != nil {
				t.Fatalf("building body: %v", marshalErr)
			}

			err := MapError(http.StatusBadRequest, body)
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), tc.secret) {
				t.Errorf("detail leaked a credential: %s", err.Error())
			}
			// The message should still say something useful.
			if !strings.Contains(err.Error(), redactedPlaceholder) {
				t.Errorf("expected a redaction marker so the message is not silently gutted: %s", err.Error())
			}
		})
	}
}

// A detail with nothing credential-shaped in it must survive intact, or the
// scrubber has made every error message useless.
func TestErrorKeepsHarmlessDetail(t *testing.T) {
	t.Parallel()

	body := []byte(`{"detail": "Flow with slug default-authorization-flow does not exist"}`)
	err := MapError(http.StatusBadRequest, body)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "default-authorization-flow") {
		t.Errorf("a harmless detail was scrubbed away: %s", err.Error())
	}
}
