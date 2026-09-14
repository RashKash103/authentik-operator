package authentik

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Supported authentik version bounds, both inclusive series in CalVer YYYY.M.
//
// These MUST stay in sync with supported-versions.yaml at the repository root,
// which is the single source of truth consumed by CI and the README. go:embed
// cannot reach a parent directory, so the bounds are duplicated here and
// TestSupportedVersionConstantsMatchYAML fails the build if they ever drift.
const (
	// MinimumVersion is the oldest authentik series this operator reconciles
	// against. Anything older is reported unsupported.
	MinimumVersion = "2026.2"

	// MaximumVersion is the newest authentik series this operator has been
	// tested against. Newer versions still reconcile, with a warning.
	MaximumVersion = "2026.8"
)

// SupportLevel describes how a running authentik version relates to the
// versions this operator was built and tested against.
type SupportLevel int

const (
	// SupportUnknown means the running version could not be parsed.
	SupportUnknown SupportLevel = iota
	// SupportUnsupported means the running version is older than MinimumVersion.
	// Controllers must refuse to reconcile rather than guess at API shapes.
	SupportUnsupported
	// SupportSupported means the running version is inside the tested range.
	SupportSupported
	// SupportUntested means the running version is newer than MaximumVersion.
	// Reconciliation proceeds, but the caller should warn.
	SupportUntested
)

// String renders the support level for logs and status conditions.
func (l SupportLevel) String() string {
	switch l {
	case SupportUnsupported:
		return "Unsupported"
	case SupportSupported:
		return "Supported"
	case SupportUntested:
		return "Untested"
	default:
		return "Unknown"
	}
}

// Version is an authentik CalVer version: a year, a month, and an optional
// patch component, as in 2026.8.2.
type Version struct {
	// Year is the CalVer year, e.g. 2026.
	Year int
	// Month is the CalVer month, 1-12. It is a number, not a string: 2026.10
	// is newer than 2026.5, which neither string nor float comparison gets right.
	Month int
	// Patch is the patch component, or 0 when the version names only a series.
	Patch int
	// Raw is the version string exactly as reported by authentik.
	Raw string
}

// ParseVersion parses an authentik CalVer version such as "2026.8", "2026.8.2"
// or "2026.8.2-rc1". Build metadata and pre-release suffixes are accepted and
// ignored for ordering.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Version{}, fmt.Errorf("%w: empty authentik version", ErrValidation)
	}

	// Drop any pre-release or build suffix: 2026.8.2-rc1+deadbeef.
	core := raw
	if i := strings.IndexAny(core, "-+ "); i >= 0 {
		core = core[:i]
	}

	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("%w: %q is not a CalVer YYYY.M[.P] version", ErrValidation, raw)
	}

	nums := make([]int, len(parts))
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return Version{}, fmt.Errorf("%w: %q is not a CalVer YYYY.M[.P] version", ErrValidation, raw)
		}
		nums[i] = n
	}
	if nums[1] < 1 || nums[1] > 12 {
		return Version{}, fmt.Errorf("%w: %q has month %d outside 1-12", ErrValidation, raw, nums[1])
	}

	v := Version{Year: nums[0], Month: nums[1], Raw: raw}
	if len(nums) == 3 {
		v.Patch = nums[2]
	}
	return v, nil
}

// Series returns the YYYY.M series this version belongs to, e.g. "2026.8".
func (v Version) Series() string {
	return strconv.Itoa(v.Year) + "." + strconv.Itoa(v.Month)
}

// String returns the version as reported by authentik, falling back to the
// normalised form when nothing was reported.
func (v Version) String() string {
	if v.Raw != "" {
		return v.Raw
	}
	return fmt.Sprintf("%d.%d.%d", v.Year, v.Month, v.Patch)
}

// Compare orders two versions component-wise, returning -1, 0 or 1. Components
// are compared as integers, so 2026.10 correctly sorts after 2026.5.
func (v Version) Compare(other Version) int {
	if c := compareInt(v.Year, other.Year); c != 0 {
		return c
	}
	if c := compareInt(v.Month, other.Month); c != 0 {
		return c
	}
	return compareInt(v.Patch, other.Patch)
}

// CompareSeries orders two versions by series only, ignoring the patch
// component. This is the comparison the support bounds use, because the bounds
// in supported-versions.yaml name series rather than exact patches.
func (v Version) CompareSeries(other Version) int {
	if c := compareInt(v.Year, other.Year); c != 0 {
		return c
	}
	return compareInt(v.Month, other.Month)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Compatibility is the verdict on a running authentik instance.
type Compatibility struct {
	// Version is the parsed running version. It is the zero Version when the
	// reported string could not be parsed.
	Version Version
	// Raw is the version string authentik reported.
	Raw string
	// Level is the support verdict.
	Level SupportLevel
	// Message is a human-readable explanation suitable for a log line or a
	// status condition.
	Message string
}

// Supported reports whether reconciliation may proceed. Both a tested version
// and a newer, untested one are allowed; only a version below MinimumVersion,
// or one that could not be parsed, is refused.
func (c Compatibility) Supported() bool {
	return c.Level == SupportSupported || c.Level == SupportUntested
}

// ShouldWarn reports whether the caller should emit a warning even though
// reconciliation may proceed.
func (c Compatibility) ShouldWarn() bool { return c.Level == SupportUntested }

// CheckVersion compares a reported authentik version string against the
// supported bounds. It never returns an error: an unparsable version is a
// verdict (SupportUnknown), not a failure of the check itself.
func CheckVersion(raw string) Compatibility {
	result := Compatibility{Raw: strings.TrimSpace(raw)}

	running, err := ParseVersion(raw)
	if err != nil {
		result.Level = SupportUnknown
		result.Message = fmt.Sprintf("authentik reported version %q, which is not a recognised CalVer version; "+
			"refusing to reconcile against an unidentified version", result.Raw)
		return result
	}
	result.Version = running

	// The bounds are compile-time constants validated by tests, so a parse
	// failure here is impossible; treat one defensively as "cannot judge".
	minimum, minErr := ParseVersion(MinimumVersion)
	maximum, maxErr := ParseVersion(MaximumVersion)
	if minErr != nil || maxErr != nil {
		result.Level = SupportUnknown
		result.Message = "supported version bounds are malformed"
		return result
	}

	switch {
	case running.CompareSeries(minimum) < 0:
		result.Level = SupportUnsupported
		result.Message = fmt.Sprintf("authentik %s is older than the minimum supported version %s",
			running, MinimumVersion)
	case running.CompareSeries(maximum) > 0:
		result.Level = SupportUntested
		result.Message = fmt.Sprintf("authentik %s is newer than the maximum tested version %s; "+
			"reconciliation will proceed but this combination is untested", running, MaximumVersion)
	default:
		result.Level = SupportSupported
		result.Message = fmt.Sprintf("authentik %s is supported (tested range %s-%s)",
			running, MinimumVersion, MaximumVersion)
	}
	return result
}

// Version reads the running authentik version from the admin version endpoint.
func (c *client) Version(ctx context.Context) (Version, error) {
	raw, err := c.rawVersion(ctx)
	if err != nil {
		return Version{}, err
	}
	return ParseVersion(raw)
}

// Compatibility reads the running authentik version and judges it against the
// supported bounds. A version that cannot be read is an error; a version that
// can be read but is unsupported is a verdict, not an error, so the caller can
// surface it on the resource instead of retrying forever.
func (c *client) Compatibility(ctx context.Context) (Compatibility, error) {
	raw, err := c.rawVersion(ctx)
	if err != nil {
		return Compatibility{}, err
	}
	return CheckVersion(raw), nil
}

// rawVersion calls GET /api/v3/admin/version/ and returns version_current.
func (c *client) rawVersion(ctx context.Context) (string, error) {
	const op = "read version"

	version, resp, err := c.api.AdminAPI.AdminVersionRetrieve(ctx).Execute()
	if err != nil {
		return "", mapResponse(op, resp, err)
	}
	if version == nil {
		return "", transientErrorf(op, "empty version response")
	}
	return version.GetVersionCurrent(), nil
}

// transientErrorf builds a transient APIError from a static message.
func transientErrorf(op, format string, args ...any) error {
	return &APIError{
		Op:     op,
		Kind:   ErrTransient,
		Detail: fmt.Sprintf(format, args...),
	}
}
