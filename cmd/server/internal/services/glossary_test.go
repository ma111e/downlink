package services

import "testing"

func TestGlossaryFromResult(t *testing.T) {
	// The per-article task extracts term/type/context only — definitions are generated
	// separately, so a term is kept whenever it has a name (no definition required).
	raw := []interface{}{
		map[string]interface{}{"term": "C2", "type": "concept", "context": "Hidden inside Teams TURN relays here."},
		map[string]interface{}{"term": "DragonForce", "type": "THREAT-ACTOR"},       // no context; type uppercased
		map[string]interface{}{"term": "Backdoor.Turn", "type": "bogus"},            // unknown type → other
		map[string]interface{}{"term": "", "context": "dropped: empty term"},        // dropped: no name
		map[string]interface{}{"term": "RAT", "context": "Used for remote access."}, // kept: no definition needed
	}

	terms := glossaryFromResult(raw)
	if len(terms) != 4 {
		t.Fatalf("expected 4 terms, got %d: %+v", len(terms), terms)
	}
	byTerm := map[string]struct{ Type, Ctx string }{}
	for _, gt := range terms {
		byTerm[gt.Term] = struct{ Type, Ctx string }{gt.Type, gt.Context}
	}
	if byTerm["C2"].Type != "concept" || byTerm["C2"].Ctx == "" {
		t.Errorf("C2 = %+v, want concept + context", byTerm["C2"])
	}
	if byTerm["DragonForce"].Type != "threat-actor" {
		t.Errorf("DragonForce type = %q, want threat-actor (lowercased)", byTerm["DragonForce"].Type)
	}
	if byTerm["Backdoor.Turn"].Type != "other" {
		t.Errorf("Backdoor.Turn type = %q, want other (unknown coerced)", byTerm["Backdoor.Turn"].Type)
	}
	if _, ok := byTerm["RAT"]; !ok {
		t.Errorf("RAT should be kept even without a definition")
	}
}

func TestEntityDefinitionsFromResult(t *testing.T) {
	raw := map[string]entityDefinition{
		"Cobalt Strike": {Def: "  A commercial pentest tool often abused by attackers.  ", Type: "tool", Difficulty: "ADVANCED"},
		"cobalt-strike": {Def: "duplicate that normalizes to the same key", Type: "TOOL", Difficulty: "advanced"},
		"unknown-thing": {Def: "", Type: "malware"},                                           // empty def: dropped (LLM didn't recognize it)
		"   ":           {Def: "x", Type: "concept"},                                          // empty key after normalization: dropped
		"DragonForce":   {Def: "A ransomware crew.", Type: "made-up-type", Difficulty: "huh"}, // unknown type/difficulty coerced
	}

	got := entityDefinitionsFromResult(raw)

	if _, ok := got["unknown-thing"]; ok {
		t.Error("empty definition should be dropped")
	}
	cs, ok := got["cobalt strike"]
	if !ok {
		t.Fatalf("expected normalized key 'cobalt strike', got %v", got)
	}
	if cs.Def == "" {
		t.Error("definition should be trimmed but non-empty")
	}
	if cs.Type != "tool" {
		t.Errorf("type = %q, want tool (coerced/lowercased)", cs.Type)
	}
	if cs.Difficulty != "advanced" {
		t.Errorf("difficulty = %q, want advanced (lowercased)", cs.Difficulty)
	}
	if df := got["dragonforce"]; df.Difficulty != "intermediate" {
		t.Errorf("unknown difficulty = %q, want intermediate (default)", df.Difficulty)
	}
	if df := got["dragonforce"]; df.Type != "other" {
		t.Errorf("unknown type = %q, want other", df.Type)
	}
	if len(got) != 2 {
		t.Errorf("expected two entries after dedup/drop, got %d: %v", len(got), got)
	}
}
