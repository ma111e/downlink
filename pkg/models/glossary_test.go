package models

import "testing"

func TestNormalizeGlossaryKey(t *testing.T) {
	// All these forms must collapse to the same key so a highlighted prose word
	// resolves to exactly one glossary entry.
	groups := map[string][]string{
		"cobalt strike": {"Cobalt Strike", "cobalt-strike", "cobalt   strike", "#cobalt strike", "  Cobalt-Strike  "},
		// Punctuation collapses too, so a tag slug and its real written form share one key.
		"wscript exe": {"wscript.exe", "wscript-exe", "WScript exe"},
		"http 3":      {"HTTP/3", "http-3", "HTTP 3"},
		"mitre att ck": {"MITRE ATT&CK", "mitre-att-ck"},
	}
	for want, forms := range groups {
		for _, in := range forms {
			if got := NormalizeGlossaryKey(in); got != want {
				t.Errorf("NormalizeGlossaryKey(%q) = %q, want %q", in, got, want)
			}
		}
	}

	cases := map[string]string{
		"":                 "",
		"C2":               "c2",
		"Lateral Movement": "lateral movement",
		"CVE-2026-0257":    "cve 2026 0257",
	}
	for in, want := range cases {
		if got := NormalizeGlossaryKey(in); got != want {
			t.Errorf("NormalizeGlossaryKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeGlossaryCategory(t *testing.T) {
	cases := map[string]string{
		"threat-actor": "threat-actor",
		"Malware":      "malware",
		"  TOOL  ":     "tool",
		"concept":      "concept",
		"":             "other",
		"made-up":      "other",
		"ransomware":   "other", // not in the taxonomy (it's a 'concept'/'malware'), coerced
	}
	for in, want := range cases {
		if got := NormalizeGlossaryCategory(in); got != want {
			t.Errorf("NormalizeGlossaryCategory(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeGlossaryDifficulty(t *testing.T) {
	cases := map[string]string{
		"beginner":     "beginner",
		"Intermediate": "intermediate",
		"  ADVANCED  ": "advanced",
		"":             "intermediate", // unset defaults to the middle bucket
		"made-up":      "intermediate",
	}
	for in, want := range cases {
		if got := NormalizeGlossaryDifficulty(in); got != want {
			t.Errorf("NormalizeGlossaryDifficulty(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGlossaryDifficultyTier(t *testing.T) {
	cases := map[string]int{
		"advanced":     1,
		"intermediate": 2,
		"beginner":     3,
		"":             2, // unset → intermediate tier
		"made-up":      2,
	}
	for in, want := range cases {
		if got := GlossaryDifficultyTier(in); got != want {
			t.Errorf("GlossaryDifficultyTier(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestEffectiveDefinition(t *testing.T) {
	e := GlossaryEntry{Definition: "generated"}
	if got := e.EffectiveDefinition(); got != "generated" {
		t.Errorf("EffectiveDefinition() = %q, want generated", got)
	}

	// A curated override wins, but only when the override flag is set and text present.
	e.CuratedDefinition = "curated"
	if got := e.EffectiveDefinition(); got != "generated" {
		t.Errorf("EffectiveDefinition() with override-off = %q, want generated", got)
	}
	e.ManualOverride = true
	if got := e.EffectiveDefinition(); got != "curated" {
		t.Errorf("EffectiveDefinition() with override-on = %q, want curated", got)
	}
}
