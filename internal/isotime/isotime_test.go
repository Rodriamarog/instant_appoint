package isotime_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Rodriamarog/instant_appoint/internal/isotime"
)

// These literals are the shape JavaScript's Date.prototype.toISOString()
// produces, which is what every timestamp already in the database looks like
// and what web/src/lib/iso-time.ts keeps producing. They are the contract:
// 24 characters, always UTC, always a literal Z, always exactly three
// fractional digits. The same instants are pinned in iso-time.test.ts.
var cases = []struct {
	name string
	in   time.Time
	want string
}{
	{
		name: "a whole second still carries three fractional digits",
		in:   time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC),
		want: "2026-11-02T17:00:00.000Z",
	},
	{
		name: "milliseconds are kept",
		in:   time.Date(2026, 11, 2, 17, 0, 0, 123_000_000, time.UTC),
		want: "2026-11-02T17:00:00.123Z",
	},
	{
		name: "a trailing zero millisecond is not trimmed",
		in:   time.Date(2026, 11, 2, 17, 0, 0, 500_000_000, time.UTC),
		want: "2026-11-02T17:00:00.500Z",
	},
	{
		name: "sub-millisecond precision is dropped, not rounded up",
		in:   time.Date(2026, 11, 2, 17, 0, 0, 123_999_999, time.UTC),
		want: "2026-11-02T17:00:00.123Z",
	},
	{
		name: "a non-UTC instant is converted rather than offset-suffixed",
		in:   time.Date(2026, 11, 2, 9, 30, 0, 0, time.FixedZone("PST", -8*3600)),
		want: "2026-11-02T17:30:00.000Z",
	},
	{
		name: "single-digit fields are zero-padded",
		in:   time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC),
		want: "2026-01-02T03:04:05.006Z",
	},
}

func TestFormatMatchesTheJavaScriptShape(t *testing.T) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isotime.Format(c.in); got != c.want {
				t.Errorf("Format() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEveryFormattedValueIsExactlyTwentyFourCharacters(t *testing.T) {
	// Fixed width is what makes a lexicographic comparison a chronological
	// one. A value of a different length sorts against the others by prefix.
	for _, c := range cases {
		if got := len(isotime.Format(c.in)); got != 24 {
			t.Errorf("Format(%v) is %d characters, want 24", c.in, got)
		}
	}
}

func TestSortingTheStringsOrdersTheInstants(t *testing.T) {
	// start_time, end_time, sent_at and last_message_at are text columns, so
	// every ordering and range filter in the app is this comparison. Go's
	// time.RFC3339 drops the fraction entirely and 'Z' (0x5A) sorts after
	// '.' (0x2E), which would put a whole second after every millisecond in
	// it; RFC3339Nano trims trailing zeros, which misorders .5 against .45.
	instants := []time.Time{
		time.Date(2026, 11, 2, 17, 0, 0, 999_000_000, time.UTC),
		time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 2, 17, 0, 0, 450_000_000, time.UTC),
		time.Date(2026, 11, 2, 17, 0, 0, 500_000_000, time.UTC),
		time.Date(2026, 11, 2, 16, 59, 59, 999_000_000, time.UTC),
	}

	formatted := make([]string, len(instants))
	for i, instant := range instants {
		formatted[i] = isotime.Format(instant)
	}
	sort.Strings(formatted)

	chronological := append([]time.Time(nil), instants...)
	sort.Slice(chronological, func(i, j int) bool { return chronological[i].Before(chronological[j]) })

	for i := range chronological {
		if want := isotime.Format(chronological[i]); formatted[i] != want {
			t.Fatalf("string sort put %q at position %d, chronological order wants %q",
				formatted[i], i, want)
		}
	}
}

func TestParseRoundTrips(t *testing.T) {
	for _, c := range cases {
		parsed, err := isotime.Parse(c.want)
		if err != nil {
			t.Fatalf("Parse(%q): %v", c.want, err)
		}
		if got := isotime.Format(parsed); got != c.want {
			t.Errorf("Format(Parse(%q)) = %q", c.want, got)
		}
	}
}

func TestParseRejectsAValueThatWouldSortWrongly(t *testing.T) {
	// These are the shapes a careless writer produces. They parse as valid
	// ISO-8601, which is exactly why they are dangerous: nothing complains
	// until an ordering silently changes. Parse is strict so a reader can
	// tell a conforming stored value from a drifted one.
	for _, bad := range []string{
		"2026-11-02T17:00:00Z",          // time.RFC3339 — no fraction
		"2026-11-02T17:00:00.5Z",        // RFC3339Nano — trimmed fraction
		"2026-11-02T17:00:00.000+00:00", // offset instead of Z
		"2026-11-02 17:00:00.000Z",      // PocketBase date-field shape
		"",
	} {
		if _, err := isotime.Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded; it should be rejected as off-format", bad)
		}
	}
}
