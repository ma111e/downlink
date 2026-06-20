package services

import "testing"

func TestGlossaryFromResult(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{"term": "C2", "type": "concept", "definition": "Command-and-control channel.", "context": "Hidden inside Teams TURN relays here."},
		map[string]interface{}{"term": "DragonForce", "type": "THREAT-ACTOR", "definition": "A ransomware crew."}, // no context; type uppercased
		map[string]interface{}{"term": "Backdoor.Turn", "type": "bogus", "definition": "A custom backdoor."},      // unknown type → other
		map[string]interface{}{"term": "", "definition": "dropped: empty term"},
		map[string]interface{}{"term": "RAT", "definition": ""}, // dropped: empty definition
	}

	terms := glossaryFromResult(raw)
	if len(terms) != 3 {
		t.Fatalf("expected 3 terms, got %d: %+v", len(terms), terms)
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
}

func TestEntityDefinitionsFromResult(t *testing.T) {
	raw := map[string]entityDefinition{
		"Cobalt Strike": {Def: "  A commercial pentest tool often abused by attackers.  ", Type: "tool"},
		"cobalt-strike": {Def: "duplicate that normalizes to the same key", Type: "TOOL"},
		"unknown-thing": {Def: "", Type: "malware"},                        // empty def: dropped (LLM didn't recognize it)
		"   ":           {Def: "x", Type: "concept"},                       // empty key after normalization: dropped
		"DragonForce":   {Def: "A ransomware crew.", Type: "made-up-type"}, // unknown type coerced to "other"
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
	if df := got["dragonforce"]; df.Type != "other" {
		t.Errorf("unknown type = %q, want other", df.Type)
	}
	if len(got) != 2 {
		t.Errorf("expected two entries after dedup/drop, got %d: %v", len(got), got)
	}
}
