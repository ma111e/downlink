package services

import "testing"

func TestEntityDefinitionsFromResult(t *testing.T) {
	raw := map[string]string{
		"Cobalt Strike": "  A commercial pentest tool often abused by attackers.  ",
		"cobalt-strike": "duplicate that normalizes to the same key",
		"unknown-thing": "",  // empty: dropped (LLM didn't recognize it)
		"   ":           "x", // empty key after normalization: dropped
	}

	got := entityDefinitionsFromResult(raw)

	if _, ok := got["unknown-thing"]; ok {
		t.Error("empty definition should be dropped")
	}
	def, ok := got["cobalt strike"]
	if !ok {
		t.Fatalf("expected normalized key 'cobalt strike', got %v", got)
	}
	if def == "" {
		t.Error("definition should be trimmed but non-empty")
	}
	if len(got) != 1 {
		t.Errorf("expected exactly one entry after dedup/drop, got %d: %v", len(got), got)
	}
}
