package notification

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30 min"},
		{90 * time.Second, "2 min"}, // rounds to nearest minute
		{20 * time.Second, "1 min"}, // floor of sub-minute is "1 min"
		{time.Hour, "1 hour"},
		{4 * time.Hour, "4 hours"},
		{25 * time.Hour, "25 hours"}, // not "1 day"
		{24 * time.Hour, "1 day"},
		{48 * time.Hour, "2 days"},
		{168 * time.Hour, "7 days"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestFormatWindowRange(t *testing.T) {
	start := time.Date(2026, 6, 26, 14, 0, 0, 0, time.UTC)

	// Same UTC day collapses the date on the end.
	if got, want := formatWindowRange(start, start.Add(4*time.Hour)), "26 Jun 14:00 → 18:00 UTC"; got != want {
		t.Errorf("same-day range = %q, want %q", got, want)
	}

	// Cross-day keeps both dates.
	if got, want := formatWindowRange(start, start.Add(24*time.Hour)), "26 Jun 14:00 → 27 Jun 14:00 UTC"; got != want {
		t.Errorf("cross-day range = %q, want %q", got, want)
	}
}

func TestFormatTimestamp(t *testing.T) {
	// Non-UTC input is normalized to UTC for display.
	loc := time.FixedZone("UTC+2", 2*60*60)
	in := time.Date(2026, 6, 26, 17, 4, 0, 0, loc)
	if got, want := formatTimestamp(in), "26 Jun 2026 15:04 UTC"; got != want {
		t.Errorf("formatTimestamp() = %q, want %q", got, want)
	}
}
