package scrapers

import (
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// dateLayouts are the formats parseItemDate tries against an element's text, in
// order. ISO and the most explicit month-name forms come first so an ambiguous
// numeric form can't shadow them. Numeric day/month is read day-first (the common
// non-US blog convention) since a bare "07/03/2024" can't be disambiguated.
var dateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"2006/01/02",
	"January 2, 2006",
	"Jan 2, 2006",
	"2 January 2006",
	"2 Jan 2006",
	"02/01/2006",
	"02-01-2006",
	"02.01.2006",
}

// parseItemDate extracts a publish date from an index-page element. It prefers a
// machine-readable datetime attribute (HTML <time datetime=...>) and otherwise
// parses the element's visible text against dateLayouts. Returns ok=false when no
// date can be read, so the caller can fall back to time.Now().
func parseItemDate(s *goquery.Selection) (time.Time, bool) {
	if s == nil || len(s.Nodes) == 0 {
		return time.Time{}, false
	}
	if dt, ok := s.Attr("datetime"); ok {
		if t, parsed := parseDateText(dt); parsed {
			return t, true
		}
	}
	return parseDateText(s.Text())
}

// parseDateText tries each layout against the trimmed text.
func parseDateText(text string) (time.Time, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
