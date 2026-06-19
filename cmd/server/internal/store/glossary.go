package store

import (
	"errors"
	"fmt"

	"github.com/ma111e/downlink/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertGlossaryEntry inserts a new glossary entry or, when an entry with the same
// NormalizedKey already exists, fills in a missing generated definition without ever
// touching the curated override. First non-empty generated definition wins (sticky).
func (s *GormStore) UpsertGlossaryEntry(entry *models.GlossaryEntry) error {
	if entry.NormalizedKey == "" {
		return fmt.Errorf("glossary entry has empty normalized key")
	}

	var existing models.GlossaryEntry
	err := s.db.Where("normalized_key = ?", entry.NormalizedKey).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := s.db.Create(entry).Error; err != nil {
			return fmt.Errorf("failed to create glossary entry: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to look up glossary entry: %w", err)
	}

	// Existing row: never touch CuratedDefinition/ManualOverride. Only backfill a
	// missing generated definition, and backfill an entity TagId when absent.
	updates := map[string]interface{}{}
	if existing.Definition == "" && entry.Definition != "" {
		updates["definition"] = entry.Definition
		updates["definition_model"] = entry.DefinitionModel
	}
	if existing.TagId == "" && entry.Kind == models.GlossaryKindEntity && entry.TagId != "" {
		updates["tag_id"] = entry.TagId
		updates["kind"] = models.GlossaryKindEntity
	}
	if len(updates) > 0 {
		if err := s.db.Model(&models.GlossaryEntry{}).Where("id = ?", existing.Id).Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update glossary entry: %w", err)
		}
	}
	// Reflect the persisted identity (notably the canonical Id) back to the caller so
	// the link table can reference it.
	entry.Id = existing.Id
	return nil
}

// GetGlossaryEntriesByKeys batch-loads existing entries for the given normalized keys,
// returned keyed by NormalizedKey. Used to skip the LLM for already-defined terms.
func (s *GormStore) GetGlossaryEntriesByKeys(keys []string) (map[string]*models.GlossaryEntry, error) {
	out := make(map[string]*models.GlossaryEntry)
	if len(keys) == 0 {
		return out, nil
	}
	var entries []models.GlossaryEntry
	if err := s.db.Where("normalized_key IN ?", keys).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("failed to load glossary entries by keys: %w", err)
	}
	for i := range entries {
		out[entries[i].NormalizedKey] = &entries[i]
	}
	return out, nil
}

// StoreDigestGlossaryBatch records the glossary entries referenced by a digest. Existing
// links are ignored (idempotent).
func (s *GormStore) StoreDigestGlossaryBatch(rows []models.DigestGlossary) error {
	if len(rows) == 0 {
		return nil
	}
	if err := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("failed to batch store digest glossary: %w", err)
	}
	return nil
}

// GetDigestGlossary loads the glossary entries referenced by a digest, with their entries.
func (s *GormStore) GetDigestGlossary(digestId string) ([]models.DigestGlossary, error) {
	var rows []models.DigestGlossary
	if err := s.db.Preload("Entry").Where("digest_id = ?", digestId).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get digest glossary: %w", err)
	}
	return rows, nil
}

// SetGlossaryManualOverride sets a curated definition that wins over and is never
// overwritten by the generated definition.
func (s *GormStore) SetGlossaryManualOverride(key, curatedDef string) error {
	key = models.NormalizeGlossaryKey(key)
	res := s.db.Model(&models.GlossaryEntry{}).Where("normalized_key = ?", key).Updates(map[string]interface{}{
		"curated_definition": curatedDef,
		"manual_override":    true,
	})
	if res.Error != nil {
		return fmt.Errorf("failed to set glossary manual override: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("glossary entry not found for key %q", key)
	}
	return nil
}
