package models

import "testing"

func TestProfileBeforeSaveSerializesJSON(t *testing.T) {
	glossary := true
	p := &Profile{
		Id:        "infosec",
		Editorial: &ProfileEditorial{Persona: "terse", Glossary: &glossary},
		Selection: &ProfileSelection{Topics: []string{"sec"}, ExcludeFeedIds: []string{"f9"}},
	}
	if err := p.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave() error = %v", err)
	}
	if p.EditorialJson == "" {
		t.Error("EditorialJson empty after BeforeSave")
	}
	if p.SelectionJson == "" {
		t.Error("SelectionJson empty after BeforeSave")
	}
}

func TestProfileAfterFindHydratesJSON(t *testing.T) {
	p := &Profile{
		EditorialJson: `{"persona":"terse","glossary":true}`,
		SelectionJson: `{"topics":["sec","ai"],"exclude_feed_ids":["f9"]}`,
	}
	if err := p.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if p.Editorial == nil || p.Editorial.Persona != "terse" {
		t.Fatalf("Editorial = %+v, want persona terse", p.Editorial)
	}
	if p.Editorial.Glossary == nil || !*p.Editorial.Glossary {
		t.Fatalf("Editorial.Glossary = %v, want true", p.Editorial.Glossary)
	}
	if p.Selection == nil || len(p.Selection.Topics) != 2 || p.Selection.Topics[1] != "ai" {
		t.Fatalf("Selection.Topics = %v, want [sec ai]", p.Selection)
	}
}

func TestProfileJSONRoundTrip(t *testing.T) {
	vibe := false
	orig := &Profile{
		Editorial: &ProfileEditorial{Provider: "openai", VibeScore: &vibe},
		Selection: &ProfileSelection{IncludeFeedIds: []string{"a", "b"}},
	}
	if err := orig.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave() error = %v", err)
	}

	// Simulate a fresh read: only the JSON columns survive a DB round-trip.
	reloaded := &Profile{EditorialJson: orig.EditorialJson, SelectionJson: orig.SelectionJson}
	if err := reloaded.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if reloaded.Editorial.Provider != "openai" {
		t.Errorf("Provider lost: %q", reloaded.Editorial.Provider)
	}
	if reloaded.Editorial.VibeScore == nil || *reloaded.Editorial.VibeScore != false {
		t.Errorf("VibeScore pointer lost: %v", reloaded.Editorial.VibeScore)
	}
	if len(reloaded.Selection.IncludeFeedIds) != 2 {
		t.Errorf("IncludeFeedIds lost: %v", reloaded.Selection.IncludeFeedIds)
	}
}

func TestProfileHooksNilFieldsAreNoOps(t *testing.T) {
	p := &Profile{} // no Editorial/Selection, empty JSON columns
	if err := p.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave() error = %v", err)
	}
	if err := p.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if p.Editorial != nil || p.Selection != nil {
		t.Fatalf("nil fields hydrated unexpectedly: %+v / %+v", p.Editorial, p.Selection)
	}
}
