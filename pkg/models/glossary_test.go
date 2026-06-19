package models

import "testing"

func TestNormalizeGlossaryKey(t *testing.T) {
	// All these forms must collapse to the same key so a highlighted prose word
	// resolves to exactly one glossary entry.
	same := []string{"Cobalt Strike", "cobalt-strike", "cobalt   strike", "#cobalt strike", "  Cobalt-Strike  "}
	want := "cobalt strike"
	for _, in := range same {
		if got := NormalizeGlossaryKey(in); got != want {
			t.Errorf("NormalizeGlossaryKey(%q) = %q, want %q", in, got, want)
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
