package scrapers

import (
	"strings"
	"testing"
)

func TestContentCoverage(t *testing.T) {
	article := "the quick brown fox jumps over the lazy dog every single morning"

	tests := []struct {
		name     string
		have     string
		want     string
		min, max float64
	}{
		{"identical is full", article, article, 1.0, 1.0},
		{"want contained in have", article + " and more trailing text here", article, 1.0, 1.0},
		{"whitespace differences ignored", strings.ReplaceAll(article, " ", "   "), article, 1.0, 1.0},
		{"disjoint text is zero", "completely different words with nothing shared at all", article, 0.0, 0.0},
		{"partial overlap is partial", "the quick brown fox went somewhere else entirely today", article, 0.05, 0.6},
		{"too short to judge", article, "two words", 0.0, 0.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ContentCoverage(tc.have, tc.want)
			if got < tc.min || got > tc.max {
				t.Errorf("ContentCoverage = %.3f, want in [%.3f, %.3f]", got, tc.min, tc.max)
			}
		})
	}
}

func TestUsable(t *testing.T) {
	long := strings.Repeat("real article words ", 100) // well over the threshold

	tests := []struct {
		name     string
		text     string
		wantOK   bool
		wantPart string // substring expected in the reason when not OK
	}{
		{"empty", "   ", false, "empty"},
		{"too short", "just a teaser", false, "too short"},
		{"anti-bot", strings.Repeat("x", 600) + " Just a moment...", false, "Cloudflare"},
		{"real content", long, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, reason := Usable(tt.text)
			if ok != tt.wantOK {
				t.Fatalf("Usable() ok = %v, want %v (reason %q)", ok, tt.wantOK, reason)
			}
			if !ok && !strings.Contains(reason, tt.wantPart) {
				t.Errorf("reason = %q, want substring %q", reason, tt.wantPart)
			}
		})
	}
}

func TestScoreSelector_AllConsistent(t *testing.T) {
	results := []ExtractResult{
		{URL: "a", Chars: 3000, Matched: true},
		{URL: "b", Chars: 3100, Matched: true},
		{URL: "c", Chars: 2900, Matched: true},
	}
	s := ScoreSelector(results)
	if s.Usable != 3 {
		t.Errorf("Usable = %d, want 3", s.Usable)
	}
	if !s.Reliable {
		t.Errorf("Score = %.2f, expected reliable (>=0.8)", s.Score)
	}
}

func TestScoreSelector_OneFluke(t *testing.T) {
	// Matches on only 1 of 5 articles: should score low and not be reliable.
	results := []ExtractResult{
		{URL: "a", Chars: 3000, Matched: true},
		{URL: "b", Chars: 0, Matched: false},
		{URL: "c", Chars: 0, Matched: false},
		{URL: "d", Chars: 0, Matched: false},
		{URL: "e", Chars: 0, Matched: false},
	}
	s := ScoreSelector(results)
	if s.Usable != 1 {
		t.Errorf("Usable = %d, want 1", s.Usable)
	}
	if s.Reliable {
		t.Errorf("Score = %.2f, expected NOT reliable for 1-of-5", s.Score)
	}
	if s.Score >= 0.5 {
		t.Errorf("Score = %.2f, want well below 0.5", s.Score)
	}
}

func TestScoreSelector_ShortContentNotUsable(t *testing.T) {
	results := []ExtractResult{
		{URL: "a", Chars: 100, Matched: true}, // below minUsableChars
		{URL: "b", Chars: 120, Matched: true},
	}
	s := ScoreSelector(results)
	if s.Usable != 0 {
		t.Errorf("Usable = %d, want 0 (both below threshold)", s.Usable)
	}
	if s.Score != 0 {
		t.Errorf("Score = %.2f, want 0", s.Score)
	}
}
