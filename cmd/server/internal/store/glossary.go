package store

import (
	"errors"
	"fmt"
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// backfillGlossaryKeys re-normalizes every glossary entry's normalized_key under the current
// NormalizeGlossaryKey rules (which collapse all punctuation, not just whitespace/hyphens),
// merging entries that now collapse to the same key. The oldest entry wins; digest references on
// the merged-away entries are repointed to it. Idempotent: a no-op once all keys are normalized.
func (s *GormStore) backfillGlossaryKeys() error {
	type row struct {
		Id            string
		NormalizedKey string
		CreatedAt     time.Time
	}
	var rows []row
	if err := s.db.Model(&models.GlossaryEntry{}).Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return fmt.Errorf("failed to load glossary entries for key backfill: %w", err)
	}

	groups := make(map[string][]row, len(rows))
	var order []string
	for _, r := range rows {
		nk := models.NormalizeGlossaryKey(r.NormalizedKey)
		if _, ok := groups[nk]; !ok {
			order = append(order, nk)
		}
		groups[nk] = append(groups[nk], r)
	}

	// Fast no-op path: skip the transaction once everything is already normalized and unique.
	work := false
	for nk, g := range groups {
		if len(g) > 1 || g[0].NormalizedKey != nk {
			work = true
			break
		}
	}
	if !work {
		return nil
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, nk := range order {
			g := groups[nk]
			keeper := g[0] // oldest, thanks to the created_at/id ordering above
			for _, loser := range g[1:] {
				// Repoint digest references to the keeper, skipping (digest_id, entry_id) pairs the
				// keeper already has (composite PK), then drop any rows the merge couldn't move.
				if err := tx.Exec("UPDATE OR IGNORE digest_glossary SET entry_id = ? WHERE entry_id = ?", keeper.Id, loser.Id).Error; err != nil {
					return fmt.Errorf("failed to repoint digest glossary rows: %w", err)
				}
				if err := tx.Exec("DELETE FROM digest_glossary WHERE entry_id = ?", loser.Id).Error; err != nil {
					return fmt.Errorf("failed to delete merged digest glossary rows: %w", err)
				}
				if err := tx.Delete(&models.GlossaryEntry{}, "id = ?", loser.Id).Error; err != nil {
					return fmt.Errorf("failed to delete merged glossary entry: %w", err)
				}
			}
			if keeper.NormalizedKey != nk {
				if err := tx.Model(&models.GlossaryEntry{}).Where("id = ?", keeper.Id).Update("normalized_key", nk).Error; err != nil {
					return fmt.Errorf("failed to update glossary entry key: %w", err)
				}
			}
		}
		return nil
	})
}

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
	if (existing.Category == "" || existing.Category == models.GlossaryCategoryOther) &&
		entry.Category != "" && entry.Category != models.GlossaryCategoryOther {
		updates["category"] = entry.Category
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

// ListGlossaryEntries returns persisted glossary entries ordered by term. limit <= 0 returns all.
func (s *GormStore) ListGlossaryEntries(limit int) ([]models.GlossaryEntry, error) {
	var entries []models.GlossaryEntry
	q := s.db.Order("term COLLATE NOCASE ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if err := q.Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("failed to list glossary entries: %w", err)
	}
	return entries, nil
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
