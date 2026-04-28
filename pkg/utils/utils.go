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
//   - Relative: "-7d", "-2h", "-30m" (days, hours, minutes ago)
func ParseTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)

	// Handle "now"
	if strings.ToLower(s) == "now" {
		return time.Now(), nil
	}

	// Handle relative time (e.g., "-7d", "-2h", "-30m")
	if strings.HasPrefix(s, "-") {
		// Parse relative duration
		durationStr := s[1:] // Remove the "-"

		var duration time.Duration
		var err error

		// Check the suffix
		if strings.HasSuffix(durationStr, "d") {
			// Days
			days := durationStr[:len(durationStr)-1]
			var daysInt int
			_, err = fmt.Sscanf(days, "%d", &daysInt)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid relative time format: %s", s)
			}
			duration = time.Duration(daysInt) * 24 * time.Hour
		} else if strings.HasSuffix(durationStr, "h") {
			// Hours
			duration, err = time.ParseDuration(durationStr)
		} else if strings.HasSuffix(durationStr, "m") {
			// Minutes
			duration, err = time.ParseDuration(durationStr)
		} else if strings.HasSuffix(durationStr, "s") {
			// Seconds
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
	t, err = time.Parse("2006-01-02", s)
	if err == nil {
		return t, nil
	}

	// Try date with time format (YYYY-MM-DD HH:MM:SS)
	t, err = time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("unable to parse time string: %s (supported formats: 'now', RFC3339, 'YYYY-MM-DD', or relative like '-7d')", s)
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
