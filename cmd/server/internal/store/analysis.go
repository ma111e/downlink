package store

import (
	"fmt"
	"downlink/pkg/models"
)

func (s *GormStore) SaveArticleAnalysis(analysis *models.ArticleAnalysis) error {

	if err := s.db.Save(analysis).Error; err != nil {
		return fmt.Errorf("failed to save article analysis: %w", err)
	}

	return nil
}

func (s *GormStore) GetArticleAnalysis(articleId string) (*models.ArticleAnalysis, error) {
	var analysis models.ArticleAnalysis

	// Get the most recent analysis for this article
	if err := s.db.Where("article_id = ?", articleId).Order("created_at DESC").First(&analysis).Error; err != nil {
		return nil, fmt.Errorf("failed to get article analysis: %w", err)
	}

	return &analysis, nil
}

// GetArticleAnalysesBatch returns the most recent analysis for each of the given article IDs.
func (s *GormStore) GetArticleAnalysesBatch(articleIds []string) (map[string]*models.ArticleAnalysis, error) {
	if len(articleIds) == 0 {
		return make(map[string]*models.ArticleAnalysis), nil
	}
	var analyses []models.ArticleAnalysis
	if err := s.db.Where("article_id IN ?", articleIds).Order("created_at DESC").Find(&analyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get article analyses batch: %w", err)
	}
	// Keep only the most recent analysis per article (already ordered DESC)
	result := make(map[string]*models.ArticleAnalysis, len(articleIds))
	for i := range analyses {
		if _, exists := result[analyses[i].ArticleId]; !exists {
			result[analyses[i].ArticleId] = &analyses[i]
		}
	}
	return result, nil
}

func (s *GormStore) GetAllArticleAnalyses(articleId string) ([]models.ArticleAnalysis, error) {
	var analyses []models.ArticleAnalysis

	if err := s.db.Where("article_id = ?", articleId).Order("created_at DESC").Find(&analyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get all article analyses: %w", err)
	}

	return analyses, nil
}
