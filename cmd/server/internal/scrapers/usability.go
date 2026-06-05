package scrapers

import (
	"fmt"
	"math"
	"strings"
)

// minUsableChars is the extracted-content length below which a scrape is treated
// as unusable (a stub, a paywall teaser, or a JS shell that never rendered).
const minUsableChars = 500

// Usable reports whether extracted article text looks like real content, with a
// short reason when it does not. It is the shared yardstick behind probe-modes and
// selector scoring, so "good enough" means the same thing everywhere.
func Usable(text string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, "empty"
	}
	if label := antiBotMarker([]byte(trimmed)); label != "" {
		return false, label
	}
	if n := len([]rune(trimmed)); n < minUsableChars {
		return false, fmt.Sprintf("too short (%d chars)", n)
	}
	return true, ""
}

// ExtractResult is one selector test against one article URL.
type ExtractResult struct {
	URL     string
	Chars   int
	Matched bool // the article selector matched an element
}

// SelectorScore summarizes how a candidate selector performed across several
// sample articles, so a selector that works on one page but flukes on others is
// not mistaken for a reliable one.
type SelectorScore struct {
	Samples   int
	Usable    int     // count of results that passed Usable
	MeanChars int     // mean extracted length over usable results
	Score     float64 // 0..1: usable ratio penalized by length variance
	Reliable  bool    // Score high enough to recommend without hesitation
}

// ScoreSelector aggregates per-article results into a stability score. The score
// is the fraction of usable samples, scaled down when usable lengths vary wildly
// (high relative variance means the selector grabs different things per page).
func ScoreSelector(results []ExtractResult) SelectorScore {
	s := SelectorScore{Samples: len(results)}
	if len(results) == 0 {
		return s
	}

	var lengths []int
	var sum int
	for _, r := range results {
		if r.Matched && r.Chars >= minUsableChars {
			s.Usable++
			lengths = append(lengths, r.Chars)
			sum += r.Chars
		}
	}

	usableRatio := float64(s.Usable) / float64(s.Samples)
	if s.Usable == 0 {
		s.Score = 0
		return s
	}

	s.MeanChars = sum / s.Usable

	// Coefficient of variation (stddev / mean) penalizes inconsistent lengths.
	var variance float64
	for _, l := range lengths {
		d := float64(l - s.MeanChars)
		variance += d * d
	}
	variance /= float64(s.Usable)
	cv := 0.0
	if s.MeanChars > 0 {
		cv = math.Sqrt(variance) / float64(s.MeanChars)
	}
	consistency := 1.0 - math.Min(cv, 1.0)

	s.Score = usableRatio * (0.5 + 0.5*consistency)
	s.Reliable = s.Score >= 0.8
	return s
}
