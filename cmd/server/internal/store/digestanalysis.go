package store

import (
	"fmt"

	"downlink/pkg/models"
)

func (s *GormStore) StoreDigestAnalysis(entry models.DigestAnalysis) error {
	if err := s.db.Save(&entry).Error; err != nil {
		return fmt.Errorf("failed to store digest analysis: %w", err)
	}
	return nil
}

func (s *GormStore) StoreDigestAnalysesBatch(entries []models.DigestAnalysis) error {
	if len(entries) == 0 {
		return nil
	}
	if err := s.db.Create(&entries).Error; err != nil {
		return fmt.Errorf("failed to batch store digest analyses: %w", err)
	}
	return nil
}

func (s *GormStore) GetDigestAnalyses(digestId string) ([]models.DigestAnalysis, error) {
	var entries []models.DigestAnalysis

	if err := s.db.Preload("Analysis").Where("digest_id = ?", digestId).Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("failed to get digest analyses: %w", err)
	}

	return entries, nil
}
