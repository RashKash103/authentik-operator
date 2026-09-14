// Package authentik provides a thin, testable client layer over the official
// authentik REST API. It is used by the operator's controllers to reconcile
// resources inside an existing authentik instance; it never deploys authentik
// itself.
package authentik

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Sentinel errors returned (wrapped) by every call in this package. Callers
// must classify failures with errors.Is or the Is* helpers below and must never
// match on error strings.
var (
	// ErrNotFound indicates the referenced object does not exist in authentik.
	// Controllers should normally requeue: the object may be created later.
	ErrNotFound = errors.New("authentik: not found")

	// ErrConflict indicates the request collided with the current state of the
	// server, for example a duplicate name or a concurrent modification.
	ErrConflict = errors.New("authentik: conflict")

	// ErrUnauthorized indicates the API token is missing, invalid, or lacks the
	// permissions required for the request.
	ErrUnauthorized = errors.New("authentik: unauthorized")

	// ErrValidation indicates authentik rejected the request payload. The
	// wrapping *APIError carries the per-field messages authentik returned.
	ErrValidation = errors.New("authentik: validation failed")

	// ErrTransient indicates a failure that is expected to succeed on retry,
	// such as a network error, a timeout, a rate limit or a 5xx response.
	ErrTransient = errors.New("authentik: transient failure")

	// ErrAmbiguous indicates a human-readable reference matched more than one
	// object. Resolving it is never guessed: the caller must disambiguate.
	ErrAmbiguous = errors.New("authentik: ambiguous reference")
)

// Limits applied when extracting detail out of an error response body. They
// bound how much attacker- or user-controlled text can reach logs and the
// status conditions of a custom resource.
const (
	maxErrorFields         = 32
	maxErrorMessages       = 8
	maxErrorMessageLength  = 512
	maxErrorResponseLength = 1 << 20 // 1 MiB
)

// redactedPlaceholder replaces any message attached to a field whose name looks
// like it carries a credential.
const redactedPlaceholder = "[redacted]"

// sensitiveFieldFragments are matched (case-insensitively, as substrings)
// against field names before their messages are kept. authentik echoes some
// submitted values back inside validation messages, so messages belonging to
// credential-bearing fields are dropped rather than surfaced.
var sensitiveFieldFragments = []string{
	"secret",
	"token",
	"password",
	"passphrase",
	"credential",
	"private_key",
	"privatekey",
	"client_id",
	"api_key",
	"apikey",
	"bind_password",
}

// APIError is the concrete error type returned for every failed authentik API
// call. It wraps exactly one of the package sentinels, which is what errors.Is
// matches against.
//
// APIError is built only from the HTTP status and from structured fields
// recognised inside the response body. The request body is never read, copied
// or wrapped, so secrets sent to authentik (client secrets, tokens, private
// keys) cannot leak into an error string, a log line or a resource condition.
type APIError struct {
	// StatusCode is the HTTP status code, or 0 when the request never
	// produced a response (for example a dial failure).
	StatusCode int

	// Op is a short, static description of the operation that failed, such as
	// "list flows". It never contains user data.
	Op string

	// Kind is the sentinel this error wraps, or nil for an unclassified status.
	Kind error

	// Detail is authentik's top-level "detail" message, when present.
	Detail string

	// Code is authentik's machine-readable error code, when present.
	Code string

	// FieldErrors holds authentik's per-field validation messages, keyed by
	// field name. The key "non_field_errors" holds object-level messages.
	// Messages for credential-bearing fields are redacted.
	FieldErrors map[string][]string
}

// Error implements error. The message is assembled only from the status code,
// the static operation name and sanitised response detail.
func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("authentik")
	if e.Op != "" {
		b.WriteString(": ")
		b.WriteString(e.Op)
	}
	if e.StatusCode != 0 {
		fmt.Fprintf(&b, ": http %d %s", e.StatusCode, strings.ToLower(http.StatusText(e.StatusCode)))
	}
	if e.Code != "" {
		fmt.Fprintf(&b, ": code=%s", e.Code)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	if fields := e.fieldSummary(); fields != "" {
		b.WriteString(": ")
		b.WriteString(fields)
	}
	if e.Detail == "" && e.Code == "" && len(e.FieldErrors) == 0 && e.Kind != nil {
		b.WriteString(": ")
		b.WriteString(e.Kind.Error())
	}
	return b.String()
}

// fieldSummary renders FieldErrors deterministically, e.g.
// "name: This field is required.; non_field_errors: ...".
func (e *APIError) fieldSummary() string {
	if len(e.FieldErrors) == 0 {
		return ""
	}
	names := make([]string, 0, len(e.FieldErrors))
	for name := range e.FieldErrors {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, name+": "+strings.Join(e.FieldErrors[name], " "))
	}
	return strings.Join(parts, "; ")
}

// Unwrap returns the sentinel this error represents so errors.Is works.
func (e *APIError) Unwrap() error { return e.Kind }

// Retryable reports whether the caller should retry the request unchanged.
func (e *APIError) Retryable() bool { return errors.Is(e.Kind, ErrTransient) }

// IsNotFound reports whether err indicates a missing object.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// IsConflict reports whether err indicates a conflicting server state.
func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }

// IsUnauthorized reports whether err indicates missing or insufficient credentials.
func IsUnauthorized(err error) bool { return errors.Is(err, ErrUnauthorized) }

// IsValidation reports whether err indicates authentik rejected the payload.
func IsValidation(err error) bool { return errors.Is(err, ErrValidation) }

// IsTransient reports whether err is expected to succeed on retry.
func IsTransient(err error) bool { return errors.Is(err, ErrTransient) }

// IsAmbiguous reports whether a human-readable reference matched several objects.
func IsAmbiguous(err error) bool { return errors.Is(err, ErrAmbiguous) }

// FieldErrors returns authentik's per-field validation messages carried by err,
// if any. The second result reports whether err carried field messages at all.
func FieldErrors(err error) (map[string][]string, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || len(apiErr.FieldErrors) == 0 {
		return nil, false
	}
	out := make(map[string][]string, len(apiErr.FieldErrors))
	for name, msgs := range apiErr.FieldErrors {
		out[name] = append([]string(nil), msgs...)
	}
	return out, true
}

// kindForStatus maps an HTTP status code onto the sentinel that best describes
// it. An unrecognised 4xx maps to nil: it is a client error, so it is neither
// retryable nor meaningfully classified.
func kindForStatus(status int) error {
	switch status {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return ErrValidation
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	case http.StatusConflict:
		return ErrConflict
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return ErrTransient
	}
	if status >= 500 {
		return ErrTransient
	}
	return nil
}

// MapError converts an HTTP status code and a response body into a typed error.
// It returns nil for any status below 300.
//
// Only recognised structures are read out of body: a top-level "detail" string,
// a "code" string, and per-field message arrays as produced by authentik's
// serializers. Anything else in the body — including a request payload echoed
// back by a misbehaving proxy — is discarded, so no secret can travel out of
// this function.
func MapError(status int, body []byte) error {
	return mapError("", status, body)
}

func mapError(op string, status int, body []byte) error {
	if status < 300 {
		return nil
	}
	apiErr := &APIError{
		StatusCode: status,
		Op:         op,
		Kind:       kindForStatus(status),
	}
	if len(body) > maxErrorResponseLength {
		body = body[:maxErrorResponseLength]
	}
	detail, code, fields := parseErrorBody(body)
	apiErr.Detail = detail
	apiErr.Code = code
	apiErr.FieldErrors = fields
	return apiErr
}

// parseErrorBody extracts the structured parts of an authentik error response.
// A body that is not a JSON object is ignored entirely rather than embedded,
// because an opaque body is exactly where an echoed request payload would hide.
func parseErrorBody(body []byte) (detail, code string, fields map[string][]string) {
	if len(body) == 0 {
		return "", "", nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", nil
	}

	for key, value := range raw {
		switch key {
		case "detail":
			var s string
			if json.Unmarshal(value, &s) == nil {
				detail = truncate(s)
			}
			continue
		case "code":
			var s string
			if json.Unmarshal(value, &s) == nil {
				code = truncate(s)
			}
			continue
		}

		// Field errors are arrays of strings. Any other shape (a bare string, a
		// number, a nested object) is dropped: authentik does not emit those as
		// validation messages, so keeping them risks echoing submitted values.
		var msgs []string
		if json.Unmarshal(value, &msgs) != nil {
			continue
		}
		if len(fields) >= maxErrorFields {
			continue
		}
		if fields == nil {
			fields = make(map[string][]string)
		}
		fields[key] = sanitizeMessages(key, msgs)
	}
	return detail, code, fields
}

// sanitizeMessages bounds and redacts the messages attached to a single field.
func sanitizeMessages(field string, msgs []string) []string {
	if isSensitiveField(field) {
		return []string{redactedPlaceholder}
	}
	if len(msgs) > maxErrorMessages {
		msgs = msgs[:maxErrorMessages]
	}
	out := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, truncate(msg))
	}
	return out
}

// isSensitiveField reports whether a field name looks like it carries a credential.
func isSensitiveField(field string) bool {
	lower := strings.ToLower(field)
	for _, fragment := range sensitiveFieldFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// truncate bounds a message to maxErrorMessageLength. Keeping an authentik
// error short matters because it ends up in a condition message, and an
// over-long status update is rejected outright.
func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxErrorMessageLength {
		return s
	}
	return s[:maxErrorMessageLength] + "…"
}

// transientError wraps a transport-level failure. The underlying error is not
// wrapped with %w on purpose for bodies: only its message is kept, and the
// generated client never places a request body in it.
func transientError(op string, err error) error {
	return &APIError{
		Op:     op,
		Kind:   ErrTransient,
		Detail: truncate(err.Error()),
	}
}

// notFoundError builds an ErrNotFound for a reference that resolved to nothing.
func notFoundError(op, kind, name string) error {
	return fmt.Errorf("%w: no %s matches %q (%s)", ErrNotFound, kind, name, op)
}

// ambiguousError builds an ErrAmbiguous for a reference that matched repeatedly.
func ambiguousError(op, kind, name string, matches int) error {
	return fmt.Errorf("%w: %d %ss match %q (%s); reference it by primary key instead",
		ErrAmbiguous, matches, kind, name, op)
}

// mapResponse converts the (response, error) pair returned by every generated
// API method into one of this package's typed errors.
//
// The generated client's own error value is deliberately dropped rather than
// wrapped: it carries the raw response body on a Body() accessor, and nothing
// downstream should be able to reach that. Only the status code and the fields
// this package recognises survive.
func mapResponse(op string, resp *http.Response, err error) error {
	if err == nil {
		return nil
	}
	if resp == nil {
		return transientError(op, err)
	}
	if resp.StatusCode < 300 {
		// A 2xx that failed to decode: the server answered but we could not
		// use the answer. Retrying is the right default.
		return transientError(op, err)
	}
	return mapError(op, resp.StatusCode, readErrorBody(resp))
}

// readErrorBody re-reads the response body. The generated client always
// restores resp.Body as an in-memory buffer after reading it, so this neither
// blocks on the network nor consumes anything the caller still needs.
func readErrorBody(resp *http.Response) []byte {
	if resp.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseLength))
	if err != nil {
		return nil
	}
	return body
}
