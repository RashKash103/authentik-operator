package authentik

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		wantYear  int
		wantMonth int
		wantPatch int
		wantErr   bool
	}{
		{name: "series only", input: "2026.8", wantYear: 2026, wantMonth: 8},
		{name: "with patch", input: "2026.8.2", wantYear: 2026, wantMonth: 8, wantPatch: 2},
		{name: "double digit month", input: "2026.12.1", wantYear: 2026, wantMonth: 12, wantPatch: 1},
		{name: "pre-release suffix", input: "2026.10.1-rc1", wantYear: 2026, wantMonth: 10, wantPatch: 1},
		{name: "build metadata", input: "2026.2.7+deadbeef", wantYear: 2026, wantMonth: 2, wantPatch: 7},
		{name: "surrounding whitespace", input: "  2026.5.7  ", wantYear: 2026, wantMonth: 5, wantPatch: 7},
		{name: "empty", input: "", wantErr: true},
		{name: "not a version", input: "unknown", wantErr: true},
		{name: "single component", input: "2026", wantErr: true},
		{name: "too many components", input: "2026.8.2.1", wantErr: true},
		{name: "month zero", input: "2026.0", wantErr: true},
		{name: "month out of range", input: "2026.13", wantErr: true},
		{name: "non numeric month", input: "2026.x", wantErr: true},
		{name: "negative", input: "2026.-1", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseVersion(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseVersion(%q) = %+v, want error", tc.input, got)
				}
				if !IsValidation(err) {
					t.Errorf("ParseVersion(%q) error is not ErrValidation: %v", tc.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseVersion(%q) returned error: %v", tc.input, err)
			}
			if got.Year != tc.wantYear || got.Month != tc.wantMonth || got.Patch != tc.wantPatch {
				t.Errorf("ParseVersion(%q) = %d.%d.%d, want %d.%d.%d",
					tc.input, got.Year, got.Month, got.Patch, tc.wantYear, tc.wantMonth, tc.wantPatch)
			}
		})
	}
}

// TestVersionCompare pins the CalVer ordering rule: components are numbers, so
// 2026.10 is newer than 2026.5 — which both float and string comparison get
// backwards.
func TestVersionCompare(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal", a: "2026.8.2", b: "2026.8.2", want: 0},
		{name: "double digit month beats single digit", a: "2026.10", b: "2026.5", want: 1},
		{name: "single digit month loses to double digit", a: "2026.9", b: "2026.12", want: -1},
		{name: "month two beats month one", a: "2026.2", b: "2026.1", want: 1},
		{name: "later year wins", a: "2027.1", b: "2026.12", want: 1},
		{name: "earlier year loses", a: "2025.12", b: "2026.1", want: -1},
		{name: "patch breaks the tie", a: "2026.8.10", b: "2026.8.9", want: 1},
		{name: "missing patch is zero", a: "2026.8", b: "2026.8.1", want: -1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, err := ParseVersion(tc.a)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.a, err)
			}
			b, err := ParseVersion(tc.b)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.b, err)
			}
			if got := a.Compare(b); got != tc.want {
				t.Errorf("%s.Compare(%s) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
			if got := b.Compare(a); got != -tc.want {
				t.Errorf("%s.Compare(%s) = %d, want %d (antisymmetry)", tc.b, tc.a, got, -tc.want)
			}
		})
	}
}

func TestVersionCompareSeriesIgnoresPatch(t *testing.T) {
	t.Parallel()

	a, _ := ParseVersion("2026.2.99")
	b, _ := ParseVersion("2026.2.0")
	if got := a.CompareSeries(b); got != 0 {
		t.Errorf("CompareSeries ignoring patch = %d, want 0", got)
	}
	if got := a.Series(); got != "2026.2" {
		t.Errorf("Series() = %q, want 2026.2", got)
	}
}

// TestCheckVersionBoundaries exercises the inclusive edges of the supported
// range, which is where an off-by-one would be invisible in production.
func TestCheckVersionBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		raw           string
		wantLevel     SupportLevel
		wantSupported bool
		wantWarn      bool
	}{
		// Cases are derived from the bounds rather than hardcoded. A previous
		// version of this test pinned 2026.2 and 2026.5 as supported, and
		// broke the moment the range narrowed - the test was asserting the
		// range of the day rather than the behaviour at its edges.
		{name: "one series below minimum", raw: seriesBelow(MinimumVersion), wantLevel: SupportUnsupported},
		{name: "a year below minimum", raw: "2025.10.1", wantLevel: SupportUnsupported},
		{name: "exactly minimum", raw: MinimumVersion, wantLevel: SupportSupported, wantSupported: true},
		{name: "minimum with patch", raw: MinimumVersion + ".7", wantLevel: SupportSupported, wantSupported: true},
		{name: "exactly maximum", raw: MaximumVersion, wantLevel: SupportSupported, wantSupported: true},
		{name: "maximum with high patch", raw: MaximumVersion + ".99", wantLevel: SupportSupported, wantSupported: true},
		{name: "one series above maximum", raw: seriesAbove(MaximumVersion), wantLevel: SupportUntested, wantSupported: true, wantWarn: true},
		{name: "double digit month above maximum", raw: "2026.12.0", wantLevel: SupportUntested, wantSupported: true, wantWarn: true},
		{name: "next year", raw: "2027.1.0", wantLevel: SupportUntested, wantSupported: true, wantWarn: true},
		{name: "unparsable", raw: "not-a-version", wantLevel: SupportUnknown},
		{name: "empty", raw: "", wantLevel: SupportUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := CheckVersion(tc.raw)
			if got.Level != tc.wantLevel {
				t.Errorf("CheckVersion(%q).Level = %v, want %v", tc.raw, got.Level, tc.wantLevel)
			}
			if got.Supported() != tc.wantSupported {
				t.Errorf("CheckVersion(%q).Supported() = %v, want %v", tc.raw, got.Supported(), tc.wantSupported)
			}
			if got.ShouldWarn() != tc.wantWarn {
				t.Errorf("CheckVersion(%q).ShouldWarn() = %v, want %v", tc.raw, got.ShouldWarn(), tc.wantWarn)
			}
			if got.Message == "" {
				t.Errorf("CheckVersion(%q).Message is empty", tc.raw)
			}
		})
	}
}

func TestSupportLevelString(t *testing.T) {
	t.Parallel()

	want := map[SupportLevel]string{
		SupportUnknown:     "Unknown",
		SupportUnsupported: "Unsupported",
		SupportSupported:   "Supported",
		SupportUntested:    "Untested",
	}
	for level, text := range want {
		if got := level.String(); got != text {
			t.Errorf("SupportLevel(%d).String() = %q, want %q", level, got, text)
		}
	}
}

// supportedVersionsFile mirrors supported-versions.yaml at the repository root.
type supportedVersionsFile struct {
	Supported []struct {
		Series string `json:"series"`
		Image  string `json:"image"`
	} `json:"supported"`
	Minimum string `json:"minimum"`
	Maximum string `json:"maximum"`
}

// TestSupportedVersionConstantsMatchYAML keeps MinimumVersion and
// MaximumVersion in sync with supported-versions.yaml, the single source of
// truth for CI and the README.
//
// The go:embed directive cannot reach outside its package directory, and the file lives at the
// repository root, so the bounds are duplicated as constants and this test is
// what stops the two from drifting.
func TestSupportedVersionConstantsMatchYAML(t *testing.T) {
	t.Parallel()

	path := filepath.Join("..", "..", "supported-versions.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var file supportedVersionsFile
	if err := yaml.Unmarshal(raw, &file); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	if file.Minimum != MinimumVersion {
		t.Errorf("supported-versions.yaml minimum = %q but MinimumVersion = %q; "+
			"update the constant in version.go", file.Minimum, MinimumVersion)
	}
	if file.Maximum != MaximumVersion {
		t.Errorf("supported-versions.yaml maximum = %q but MaximumVersion = %q; "+
			"update the constant in version.go", file.Maximum, MaximumVersion)
	}
	if len(file.Supported) == 0 {
		t.Fatal("supported-versions.yaml lists no supported series")
	}

	minimum, err := ParseVersion(MinimumVersion)
	if err != nil {
		t.Fatalf("MinimumVersion %q does not parse: %v", MinimumVersion, err)
	}
	maximum, err := ParseVersion(MaximumVersion)
	if err != nil {
		t.Fatalf("MaximumVersion %q does not parse: %v", MaximumVersion, err)
	}
	if minimum.CompareSeries(maximum) > 0 {
		t.Fatalf("MinimumVersion %q is newer than MaximumVersion %q", MinimumVersion, MaximumVersion)
	}

	// Every series CI tests must fall inside the range the runtime gate accepts.
	for _, entry := range file.Supported {
		series, err := ParseVersion(entry.Series)
		if err != nil {
			t.Errorf("supported series %q does not parse: %v", entry.Series, err)
			continue
		}
		if c := CheckVersion(entry.Series); !c.Supported() || c.ShouldWarn() {
			t.Errorf("series %q from supported-versions.yaml is judged %v by the runtime gate, want Supported",
				entry.Series, c.Level)
		}
		if series.CompareSeries(minimum) < 0 || series.CompareSeries(maximum) > 0 {
			t.Errorf("series %q lies outside the constant bounds %s-%s",
				entry.Series, MinimumVersion, MaximumVersion)
		}
	}
}

// seriesBelow returns the CalVer series immediately before the given one.
func seriesBelow(series string) string {
	return shiftSeries(series, -1)
}

// seriesAbove returns the CalVer series immediately after the given one.
func seriesAbove(series string) string {
	return shiftSeries(series, 1)
}

// shiftSeries moves a "YYYY.M" series by whole months, rolling the year over.
func shiftSeries(series string, delta int) string {
	var year, month int
	if _, err := fmt.Sscanf(series, "%d.%d", &year, &month); err != nil {
		panic("unparsable series in test: " + series)
	}
	month += delta
	switch {
	case month < 1:
		year, month = year-1, 12
	case month > 12:
		year, month = year+1, 1
	}
	return fmt.Sprintf("%d.%d.0", year, month)
}
