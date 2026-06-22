package services

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

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

func TestRemapEntityDefsByID(t *testing.T) {
	// defineEntities addresses terms by synthetic id; the model may reword/expand the term
	// text but must key its answer by the id. Remapping recovers the original input term so
	// the downstream normalized key matches the tag candidate (the bug this guards against).
	idToTerm := map[string]string{
		"t1": "mitre-attack",
		"t2": "mcp",
		"t3": "lazarus",
	}
	byID := map[string]entityDefinition{
		"t1":  {Def: "An adversary tactics knowledge base.", Type: "concept"}, // model reworded term as "MITRE ATT&CK"
		" T2": {Def: "A protocol for tool/model integration.", Type: "protocol"}, // id whitespace/case tolerated
		"t9":  {Def: "hallucinated id, no such term", Type: "tool"},              // unknown id: dropped
	}

	got := remapEntityDefsByID(byID, idToTerm)

	if _, ok := got["t9"]; ok {
		t.Error("unknown id should be dropped")
	}
	if d, ok := got["mitre-attack"]; !ok || d.Def == "" {
		t.Errorf("t1 should map back to 'mitre-attack', got %+v", got)
	}
	if _, ok := got["mcp"]; !ok {
		t.Errorf("' T2' should map back to 'mcp' (id trimmed/lowercased), got %+v", got)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries (t1, t2), got %d: %+v", len(got), got)
	}

	// End-to-end through entityDefinitionsFromResult: the recovered original term must
	// normalize to the candidate key, not to whatever the model echoed.
	defs := entityDefinitionsFromResult(got)
	if _, ok := defs[models.NormalizeGlossaryKey("mitre-attack")]; !ok {
		t.Errorf("definition should be keyed by normalized 'mitre-attack', got %v", defs)
	}
}

func TestArticleTermContextsFromResult(t *testing.T) {
	raw := map[string]string{
		"Lazarus Group": "  Runs the intrusion described in the article.  ", // trimmed
		"lazarus-group": "duplicate normalizing to the same key",            // collapses onto the key
		"QNAP NAS":      "",                                                 // empty: dropped
		"   ":           "key empties after normalization",                  // dropped
	}

	got := articleTermContextsFromResult(raw)

	if _, ok := got["qnap nas"]; ok {
		t.Error("empty context should be dropped")
	}
	v, ok := got["lazarus group"]
	if !ok {
		t.Fatalf("expected normalized key 'lazarus group', got %v", got)
	}
	if v == "" || v[0] == ' ' {
		t.Errorf("context should be trimmed and non-empty, got %q", v)
	}
	if len(got) != 1 {
		t.Errorf("expected one entry after dedup/drop, got %d: %v", len(got), got)
	}
}

func TestMergeArticleContexts(t *testing.T) {
	existing := []models.GlossaryTerm{
		{Term: "C2", Type: "concept", Context: "already has context"},
	}
	add := map[string]glossaryTermCtx{
		"lazarus group": {Term: "Lazarus Group", Category: "threat-actor", Context: "Runs the intrusion."},
		"c2":            {Term: "C2", Category: "concept", Context: "should NOT overwrite existing"},
		"empty":         {Term: "Empty", Category: "tool", Context: "  "}, // blank context: skipped
	}

	merged := mergeArticleContexts(existing, add)

	if len(merged) != 2 {
		t.Fatalf("expected 2 terms (1 existing + 1 new), got %d: %+v", len(merged), merged)
	}
	byKey := map[string]models.GlossaryTerm{}
	for _, t := range merged {
		byKey[models.NormalizeGlossaryKey(t.Term)] = t
	}
	if byKey["c2"].Context != "already has context" {
		t.Errorf("existing context must be preserved, got %q", byKey["c2"].Context)
	}
	laz, ok := byKey["lazarus group"]
	if !ok || laz.Context != "Runs the intrusion." || laz.Type != "threat-actor" {
		t.Errorf("new entity term not merged correctly: %+v", laz)
	}

	// Re-merging the same additions must not duplicate rows (idempotent across repeat digests).
	again := mergeArticleContexts(merged, add)
	if len(again) != len(merged) {
		t.Errorf("re-merge added duplicates: %d vs %d", len(again), len(merged))
	}
}
