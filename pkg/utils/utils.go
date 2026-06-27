package utils

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ParseTimeString parses a user-friendly time string into a time.Time
// Supported formats:
//   - "now" - current time
//   - RFC3339: "2006-01-02T15:04:05Z07:00"
//   - Date: "2006-01-02"
//   - Relative: "7d", "2h", "30m" or "-7d", "-2h", "-30m" (days, hours, minutes ago)
func ParseTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	// Handle "now"
	if strings.ToLower(s) == "now" {
		return time.Now(), nil
	}

	// Handle relative time (e.g., "7d", "-2h", "30m")
	durationStr := strings.TrimPrefix(s, "-")
	isRelative := durationStr != s || // had a leading "-"
		strings.HasSuffix(s, "d") ||
		strings.HasSuffix(s, "h") ||
		strings.HasSuffix(s, "m") ||
		strings.HasSuffix(s, "s")

	if isRelative {
		var duration time.Duration
		var err error

		if strings.HasSuffix(durationStr, "d") {
			days := durationStr[:len(durationStr)-1]
			var daysInt int
			_, err = fmt.Sscanf(days, "%d", &daysInt)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid relative time format: %s", s)
			}
			duration = time.Duration(daysInt) * 24 * time.Hour
		} else if strings.HasSuffix(durationStr, "h") {
			duration, err = time.ParseDuration(durationStr)
		} else if strings.HasSuffix(durationStr, "m") {
			duration, err = time.ParseDuration(durationStr)
		} else if strings.HasSuffix(durationStr, "s") {
			duration, err = time.ParseDuration(durationStr)
		} else {
			return time.Time{}, fmt.Errorf("invalid relative time format: %s (use d, h, m, or s suffix)", s)
		}

		if err != nil {
			return time.Time{}, fmt.Errorf("invalid relative time: %v", err)
		}

		return time.Now().Add(-duration), nil
	}

	// Try RFC3339 format
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t, nil
	}

	// Try date-only format (YYYY-MM-DD)
	t, err = time.ParseInLocation("2006-01-02", s, time.Local)
	if err == nil {
		return t, nil
	}

	// Try date with time format (YYYY-MM-DD HH:MM:SS)
	t, err = time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time string: %s (supported formats: 'now', RFC3339, 'YYYY-MM-DD', or relative like '7d')", s)
}

// ParseDayUTC parses a single-day selector into a [start, end) window covering
// that whole day in UTC (midnight to midnight). Accepts "YYYY-MM-DD",
// "today", or "yesterday". For the "today"/"yesterday" shortcuts the calendar
// date is taken from local time, while the returned window is still UTC.
func ParseDayUTC(s string) (start, end time.Time, err error) {
	s = strings.TrimSpace(s)
	var day time.Time
	switch strings.ToLower(s) {
	case "today":
		now := time.Now()
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "yesterday":
		now := time.Now().AddDate(0, 0, -1)
		day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	default:
		day, err = time.ParseInLocation("2006-01-02", s, time.UTC)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid --day value %q (use YYYY-MM-DD, 'today', or 'yesterday')", s)
		}
	}
	return day, day.AddDate(0, 0, 1), nil
}

// StripCSSComments removes /* ... */ comments from a stylesheet, replacing each
// with a single space (so adjacent tokens like "0/* */0" stay separated rather
// than merging into "00"), then trims trailing whitespace per line and drops any
// line left blank. It scans character by character and ignores /* sequences that
// appear inside single- or double-quoted strings (e.g. url("...")), so it never
// cuts into a value.
//
// Source .css files keep their comments; this is applied at render time so the
// published HTML stays lean.
func StripCSSComments(css string) string {
	var b strings.Builder
	b.Grow(len(css))

	var quote byte // 0 when not inside a string, else the opening quote char
	for i := 0; i < len(css); i++ {
		c := css[i]

		if quote != 0 {
			b.WriteByte(c)
			// A backslash escapes the next char inside a string literal.
			if c == '\\' && i+1 < len(css) {
				i++
				b.WriteByte(css[i])
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}

		if c == '\'' || c == '"' {
			quote = c
			b.WriteByte(c)
			continue
		}

		if c == '/' && i+1 < len(css) && css[i+1] == '*' {
			// Skip to the closing */ (or end of input for an unterminated comment).
			end := strings.Index(css[i+2:], "*/")
			if end < 0 {
				i = len(css)
			} else {
				i += 2 + end + 1 // advance past the closing "*/"
			}
			b.WriteByte(' ')
			continue
		}

		b.WriteByte(c)
	}

	// Trim trailing whitespace per line and drop lines left empty.
	lines := strings.Split(b.String(), "\n")
	out := lines[:0]
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// NormalizeFeedName converts a feed title to a URL-safe normalized name
// Converts to lowercase, replaces spaces with '-', and special chars with '_'
func NormalizeFeedName(title string) string {
	// Convert to lowercase
	normalized := strings.ToLower(title)

	// Replace spaces with hyphens
	normalized = strings.ReplaceAll(normalized, " ", "-")

	// Replace special characters (anything that's not alphanumeric or hyphen) with underscore
	re := regexp.MustCompile(`[^a-z0-9-]+`)
	normalized = re.ReplaceAllString(normalized, "_")

	// Remove leading/trailing underscores or hyphens
	normalized = strings.Trim(normalized, "_-")

	return normalized
}
