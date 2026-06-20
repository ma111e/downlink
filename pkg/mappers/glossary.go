package mappers

import (
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
)

// GlossaryEntryToProto maps a persistent glossary entry to its proto form, including the
// resolved effective definition (curated override when present, else generated).
func GlossaryEntryToProto(e *models.GlossaryEntry) *protos.GlossaryEntry {
	if e == nil {
		return nil
	}
	return &protos.GlossaryEntry{
		Id:                  e.Id,
		NormalizedKey:       e.NormalizedKey,
		Term:                e.Term,
		Kind:                string(e.Kind),
		Category:            e.Category,
		Definition:          e.Definition,
		CuratedDefinition:   e.CuratedDefinition,
		ManualOverride:      e.ManualOverride,
		TagId:               e.TagId,
		EffectiveDefinition: e.EffectiveDefinition(),
	}
}

// AllGlossaryEntriesToProto maps a slice of glossary entries to proto.
func AllGlossaryEntriesToProto(entries []models.GlossaryEntry) []*protos.GlossaryEntry {
	out := make([]*protos.GlossaryEntry, 0, len(entries))
	for i := range entries {
		out = append(out, GlossaryEntryToProto(&entries[i]))
	}
	return out
}

// GlossaryEntryToModel maps a proto glossary entry back to the model form.
func GlossaryEntryToModel(e *protos.GlossaryEntry) *models.GlossaryEntry {
	if e == nil {
		return nil
	}
	return &models.GlossaryEntry{
		Id:                e.Id,
		NormalizedKey:     e.NormalizedKey,
		Term:              e.Term,
		Kind:              models.GlossaryKind(e.Kind),
		Category:          e.Category,
		Definition:        e.Definition,
		CuratedDefinition: e.CuratedDefinition,
		ManualOverride:    e.ManualOverride,
		TagId:             e.TagId,
	}
}

// AllGlossaryEntriesToModels maps a slice of proto glossary entries back to models.
func AllGlossaryEntriesToModels(entries []*protos.GlossaryEntry) []models.GlossaryEntry {
	out := make([]models.GlossaryEntry, 0, len(entries))
	for _, e := range entries {
		if m := GlossaryEntryToModel(e); m != nil {
			out = append(out, *m)
		}
	}
	return out
}
