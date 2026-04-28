package store

import (
	"fmt"
	"downlink/pkg/models"
)

func (s *GormStore) StoreDigestProviderResult(result models.DigestProviderResult) error {
	if err := s.db.Save(&result).Error; err != nil {
		return fmt.Errorf("failed to store digest provider result: %w", err)
	}
	return nil
}

func (s *GormStore) GetDigestProviderResults(digestId string) ([]models.DigestProviderResult, error) {
	var results []models.DigestProviderResult

	if err := s.db.Where("digest_id = ?", digestId).Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get digest provider results: %w", err)
	}

	return results, nil
}

func (s *GormStore) GetDigestProviderResult(id string) (models.DigestProviderResult, error) {
	var result models.DigestProviderResult

	if err := s.db.First(&result, "id = ?", id).Error; err != nil {
		return result, fmt.Errorf("failed to get digest provider result: %w", err)
	}

	return result, nil
}
