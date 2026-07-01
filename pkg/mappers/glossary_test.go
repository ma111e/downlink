package mappers

import (
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestGlossaryEntryRoundTripPreservesFields(t *testing.T) {
	in := &models.GlossaryEntry{
		Id:                "e1",
		NormalizedKey:     "cobalt strike",
		Term:              "Cobalt Strike",
		Kind:              models.GlossaryKindEntity,
		Category:          "tool",
		Difficulty:        "intermediate",
		Definition:        "generated definition",
		CuratedDefinition: "",
		ManualOverride:    false,
		TagId:             "cobalt-strike",
	}
	proto := GlossaryEntryToProto(in)
	out := GlossaryEntryToModel(proto)
	if out == nil {
		t.Fatal("GlossaryEntryToModel returned nil")
	}
	if out.Id != "e1" || out.NormalizedKey != "cobalt strike" || out.Term != "Cobalt Strike" {
		t.Errorf("id/key/term lost: %+v", out)
	}
	if string(out.Kind) != "entity" || out.Category != "tool" || out.Difficulty != "intermediate" {
		t.Errorf("kind/category/difficulty lost: %+v", out)
	}
	if out.Definition != "generated definition" || out.TagId != "cobalt-strike" {
		t.Errorf("definition/tagId lost: %+v", out)
	}
}

func TestGlossaryEntryToProtoUsesEffectiveDefinition(t *testing.T) {
	withOverride := &models.GlossaryEntry{
		Definition:        "generated",
		CuratedDefinition: "curated",
		ManualOverride:    true,
	}
	proto := GlossaryEntryToProto(withOverride)
	if proto.EffectiveDefinition != "curated" {
		t.Errorf("EffectiveDefinition = %q with override, want \"curated\"", proto.EffectiveDefinition)
	}

	withoutOverride := &models.GlossaryEntry{
		Definition:        "generated",
		CuratedDefinition: "curated",
		ManualOverride:    false,
	}
	proto2 := GlossaryEntryToProto(withoutOverride)
	if proto2.EffectiveDefinition != "generated" {
		t.Errorf("EffectiveDefinition = %q without override, want \"generated\"", proto2.EffectiveDefinition)
	}
}

func TestGlossaryEntryToProtoNilIsNil(t *testing.T) {
	if GlossaryEntryToProto(nil) != nil {
		t.Fatal("GlossaryEntryToProto(nil) != nil")
	}
}

func TestGlossaryEntryToModelNilIsNil(t *testing.T) {
	if GlossaryEntryToModel(nil) != nil {
		t.Fatal("GlossaryEntryToModel(nil) != nil")
	}
}

func TestAllGlossaryEntriesRoundTrip(t *testing.T) {
	in := []models.GlossaryEntry{
		{Id: "e1", Term: "APT", NormalizedKey: "apt", Kind: models.GlossaryKindEntity, Definition: "def1"},
		{Id: "e2", Term: "IOC", NormalizedKey: "ioc", Kind: models.GlossaryKindJargon, Definition: "def2"},
	}
	out := AllGlossaryEntriesToModels(AllGlossaryEntriesToProto(in))
	if len(out) != 2 || out[0].Term != "APT" || out[1].Term != "IOC" {
		t.Errorf("slice round-trip lost data: %+v", out)
	}
}

func TestAllGlossaryEntriesToModelsSkipsNil(t *testing.T) {
	protos := AllGlossaryEntriesToProto([]models.GlossaryEntry{{Term: "X", NormalizedKey: "x"}})
	protos = append(protos, nil)
	out := AllGlossaryEntriesToModels(protos)
	if len(out) != 1 {
		t.Errorf("len = %d, want 1 (nil should be skipped)", len(out))
	}
}
