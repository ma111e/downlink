package services

import (
	"context"
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

func TestGlossaryFromResultAliases(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"term": "Cisco Unified Communications Manager",
			"type": "product",
			"aliases": []interface{}{
				"  Unified CM  ",                       // trimmed
				"Unified Communications Manager",       // kept
				"unified-cm",                           // dup of "Unified CM" by normalized key → dropped
				"Cisco Unified Communications Manager", // self-alias → dropped
				"",                                     // empty → dropped
				42,                                     // non-string → dropped
				"Call Manager",                         // kept
				"CUCM",                                 // would be 4th distinct, but cap already reached? no: this is #4
				"extra alias",                          // beyond cap of 4 → dropped
			},
		},
		map[string]interface{}{"term": "HuiOne Group", "type": "organization"}, // no aliases key → nil
	}

	terms := glossaryFromResult(raw)
	if len(terms) != 2 {
		t.Fatalf("expected 2 terms, got %d: %+v", len(terms), terms)
	}

	cucm := terms[0]
	want := []string{"Unified CM", "Unified Communications Manager", "Call Manager", "CUCM"}
	if len(cucm.Aliases) != len(want) {
		t.Fatalf("CUCM aliases = %v, want %v", cucm.Aliases, want)
	}
	for i, w := range want {
		if cucm.Aliases[i] != w {
			t.Errorf("alias[%d] = %q, want %q (full=%v)", i, cucm.Aliases[i], w, cucm.Aliases)
		}
	}

	if terms[1].Aliases != nil {
		t.Errorf("HuiOne Group aliases = %v, want nil (no aliases key)", terms[1].Aliases)
	}
}

func TestEntityDefinitionsFromResult(t *testing.T) {
	raw := map[string]entityDefinition{
		"Cobalt Strike": {Def: "  A commercial pentest tool often abused by attackers.  ", Type: "tool", Difficulty: "ADVANCED"},
		"cobalt-strike": {Def: "duplicate that normalizes to the same key", Type: "TOOL", Difficulty: "advanced"},
		"unknown-thing": {Def: "", Type: "malware"},                                           // empty def: dropped (LLM didn't recognize it)
		"   ":           {Def: "x", Type: "concept"},                                          // empty key after normalization: dropped
		"DragonForce":   {Def: "A ransomware crew.", Type: "made-up-type", Difficulty: "huh"}, // unknown type/difficulty coerced
		"wscript-exe":   {Name: "  wscript.exe  ", Def: "A Windows scripting host binary.", Type: "tool"}, // display name carried + trimmed
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
	if w := got["wscript exe"]; w.Name != "wscript.exe" {
		t.Errorf("display name = %q, want %q (trimmed, real written form preserved)", w.Name, "wscript.exe")
	}
	if len(got) != 3 {
		t.Errorf("expected three entries after dedup/drop, got %d: %v", len(got), got)
	}
}

func TestCVETagPattern(t *testing.T) {
	match := []string{"cve-2026-20230", "CVE-2026-0257", "cve-2024-1", "cve 2026 20230"}
	for _, s := range match {
		if !cveTagPattern.MatchString(s) {
			t.Errorf("cveTagPattern should match %q", s)
		}
	}
	noMatch := []string{"cve", "cobalt-strike", "cve-2026", "microsoft-cve-thing", "cvelist", "cve-abc-123"}
	for _, s := range noMatch {
		if cveTagPattern.MatchString(s) {
			t.Errorf("cveTagPattern should not match %q", s)
		}
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

func TestMatchStoredGlossaryTerms(t *testing.T) {
	def := "A definition."
	entries := map[string]*models.GlossaryEntry{
		"cve":           {Id: "e-cve", NormalizedKey: "cve", Term: "CVE", Category: "vulnerability", Definition: def},
		"cobalt strike": {Id: "e-cs", NormalizedKey: "cobalt strike", Term: "Cobalt Strike", Category: "tool", Definition: def},
		"mitre attack":  {Id: "e-mitre", NormalizedKey: "mitre attack", Term: "MITRE ATT&CK", Category: "concept", Definition: def},
		"phishing":      {Id: "e-phish", NormalizedKey: "phishing", Term: "Phishing", Category: "technique", Definition: def},
		"nodef":         {Id: "e-nodef", NormalizedKey: "nodef", Term: "NoDef", Category: "tool", Definition: ""}, // undefined: never matched
	}
	analyses := []models.ArticleAnalysis{
		{
			Tldr:      "A new cve affects routers.", // case-insensitive
			KeyPoints: []string{"Attackers deployed cobalt-strike beacons."}, // separator variant
		},
		{
			// Display term "MITRE ATT&CK" normalizes to "mitre att ck", not the entry key
			// "mitre attack" — the match must still resolve to the entry.
			StandardSynthesis: "Techniques were mapped to MITRE ATT&CK. NoDef appears but stays out. CVEs keep rising.",
		},
	}

	got := matchStoredGlossaryTerms(analyses, entries)

	if !got[0]["cve"] || !got[0]["cobalt strike"] {
		t.Errorf("analysis 0 should match cve + cobalt strike, got %v", got[0])
	}
	if len(got[0]) != 2 {
		t.Errorf("analysis 0 matches = %v, want exactly {cve, cobalt strike}", got[0])
	}
	if !got[1]["mitre attack"] {
		t.Errorf("analysis 1 should resolve 'MITRE ATT&CK' back to entry key 'mitre attack', got %v", got[1])
	}
	if got[1]["nodef"] {
		t.Error("entries without an effective definition must be skipped")
	}
	if got[1]["cve"] {
		t.Error("'CVEs' must not match 'CVE' (word boundary)")
	}
	for i, keys := range got {
		if keys["phishing"] {
			t.Errorf("analysis %d matched 'phishing' which appears nowhere", i)
		}
	}
}

func TestMatchStoredGlossaryTermsStoplist(t *testing.T) {
	// Single-word terms that are common English words ("Go", "Signal", "Black") must never
	// prose-match: globally they are far more often the ordinary word than the glossary
	// entity. Domain words, acronyms, and multi-word terms keep matching.
	def := "A definition."
	entries := map[string]*models.GlossaryEntry{
		"go":            {Id: "e-go", NormalizedKey: "go", Term: "Go", Category: "product", Definition: def},
		"signal":        {Id: "e-signal", NormalizedKey: "signal", Term: "Signal", Category: "product", Definition: def},
		"alerts":        {Id: "e-alerts", NormalizedKey: "alerts", Term: "alerts", Category: "concept", Definition: def}, // plural of a common word
		"phishing":      {Id: "e-phish", NormalizedKey: "phishing", Term: "phishing", Category: "technique", Definition: def},
		"python":        {Id: "e-py", NormalizedKey: "python", Term: "Python", Category: "tool", Definition: def},
		"patch tuesday": {Id: "e-pt", NormalizedKey: "patch tuesday", Term: "Patch Tuesday", Category: "concept", Definition: def}, // multi-word with a stoplisted word
		"cve":           {Id: "e-cve", NormalizedKey: "cve", Term: "CVE", Category: "vulnerability", Definition: def},
	}
	analyses := []models.ArticleAnalysis{{
		Tldr: "Attackers go after Signal users; alerts fired. A phishing kit written in Python landed before Patch Tuesday, exploiting an old CVE.",
	}}

	got := matchStoredGlossaryTerms(analyses, entries)

	for _, blocked := range []string{"go", "signal", "alerts"} {
		if got[0][blocked] {
			t.Errorf("common word %q must not prose-match", blocked)
		}
	}
	for _, kept := range []string{"phishing", "python", "patch tuesday", "cve"} {
		if !got[0][kept] {
			t.Errorf("%q should prose-match, got %v", kept, got[0])
		}
	}
}

func TestCanonicalRefinesKey(t *testing.T) {
	cases := []struct {
		key, termKey string
		want         bool
	}{
		{"worker", "cloudflare workers", true}, // generic surface, specific canonical → re-key
		{"copilot", "microsoft 365 copilot", true},
		{"python black", "black", false},          // canonical is a shortening → keep surface key
		{"att", "at t", false},                    // tokenization variant, no word prefix-match
		{"mitre attack", "mitre att ck", false},   // "attack" is not a prefix of "att"
		{"cve", "cve", false},                     // identical
		{"apple macos", "macos", false},           // fewer words
		{"worker", "", false},                     // empty canonical
		{"", "cloudflare workers", false},         // empty key
		{"gh0strat", "gh0st rat", false},          // "gh0strat" is not a prefix of "gh0st"
		{"us senate", "u s senate", false},        // "us" is not a prefix of "u"
		{"unifi os", "ubiquiti unifi os", true},   // every key word prefix-matches a distinct canonical word
	}
	for _, c := range cases {
		if got := canonicalRefinesKey(c.key, c.termKey); got != c.want {
			t.Errorf("canonicalRefinesKey(%q, %q) = %v, want %v", c.key, c.termKey, got, c.want)
		}
	}
}

func TestArticleContextTerms(t *testing.T) {
	termMeta := map[string]glossaryTermCtx{
		"lazarus":       {Term: "Lazarus", Category: "threat-actor"},
		"cve":           {Term: "CVE", Category: "vulnerability"},
		"cobalt strike": {Term: "Cobalt Strike", Category: "tool"},
		"c2":            {Term: "C2", Category: "concept"},
	}
	tags := []models.Tag{
		{Name: "lazarus"},       // entity tag, no context yet → queued
		{Name: "c2"},            // has context already → skipped
		{Name: "random-tag"},    // not a glossary entry → skipped
		{Name: "cobalt-strike"}, // also prose-matched → queued once
	}
	matched := map[string]bool{
		"cve":           true, // prose-matched, no context yet → queued (display term from meta)
		"c2":            true, // has context already → skipped
		"cobalt strike": true,
	}
	existing := []models.GlossaryTerm{
		{Term: "C2", Type: "concept", Context: "already explained here"},
	}

	got := articleContextTerms(tags, matched, existing, termMeta)

	want := map[string]bool{"lazarus": true, "cobalt-strike": true, "CVE": true}
	if len(got) != len(want) {
		t.Fatalf("terms = %v, want exactly %v", got, want)
	}
	for _, term := range got {
		if !want[term] {
			t.Errorf("unexpected term %q in %v", term, got)
		}
	}
}

func TestPopulateGlossaryLinksStoredTermsFromProse(t *testing.T) {
	// The regression this guards: a term stored by a previous generation must be linked to a
	// new digest when it appears in the prose, even though this run's extraction did not
	// re-emit it (no GlossaryTerms, no tags) — and without any LLM call.
	s := withTempStore(t)
	entry := &models.GlossaryEntry{
		NormalizedKey: "cve",
		Term:          "CVE",
		Kind:          models.GlossaryKindJargon,
		Category:      "vulnerability",
		Difficulty:    "beginner",
		Definition:    "A public identifier for a known security flaw.",
	}
	if err := s.UpsertGlossaryEntry(entry); err != nil {
		t.Fatalf("seed glossary entry: %v", err)
	}

	analyses := []models.ArticleAnalysis{{ArticleId: "a1", Tldr: "A critical CVE in routers is being exploited."}}
	srv := &DigestServer{}
	n, err := srv.populateGlossary(context.Background(), "digest-test", analyses, map[string]models.Article{}, "prov", "model", EffectiveEditorial{})
	if err != nil {
		t.Fatalf("populateGlossary() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("linked entries = %d, want 1", n)
	}
	links, err := s.GetDigestGlossary("digest-test")
	if err != nil {
		t.Fatalf("GetDigestGlossary() error = %v", err)
	}
	if len(links) != 1 || links[0].EntryId != entry.Id {
		t.Errorf("digest_glossary = %+v, want single link to %s", links, entry.Id)
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
