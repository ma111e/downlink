package store

import (
	"fmt"
	"github.com/ma111e/downlink/pkg/models"
)

func (s *GormStore) SaveArticleAnalysis(analysis *models.ArticleAnalysis) error {

	if err := s.db.Save(analysis).Error; err != nil {
		return fmt.Errorf("failed to save article analysis: %w", err)
	}

	return nil
}

// GetArticleAnalysis returns the most recent analysis for an article. When
// profileId is set it returns the most recent analysis produced for that
// profile; an empty profileId returns the latest across all profiles,
// preserving the historical single-tenant behavior.
func (s *GormStore) GetArticleAnalysis(articleId, profileId string) (*models.ArticleAnalysis, error) {
	var analysis models.ArticleAnalysis

	query := s.db.Where("article_id = ?", articleId)
	if profileId != "" {
		query = query.Where("profile_id = ?", profileId)
	}
	if err := query.Order("created_at DESC").First(&analysis).Error; err != nil {
		return nil, fmt.Errorf("failed to get article analysis: %w", err)
	}

	return &analysis, nil
}

// GetArticleAnalysesBatch returns the most recent analysis for each of the given
// article IDs. When profileId is set, only that profile's analyses are
// considered; an empty profileId considers all profiles (historical behavior).
func (s *GormStore) GetArticleAnalysesBatch(articleIds []string, profileId string) (map[string]*models.ArticleAnalysis, error) {
	if len(articleIds) == 0 {
		return make(map[string]*models.ArticleAnalysis), nil
	}
	query := s.db.Where("article_id IN ?", articleIds)
	if profileId != "" {
		query = query.Where("profile_id = ?", profileId)
	}
	var analyses []models.ArticleAnalysis
	if err := query.Order("created_at DESC").Find(&analyses).Error; err != nil {
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

// GetAllArticleAnalyses returns every analysis for an article, newest first.
// When profileId is set it returns only that profile's analyses; an empty
// profileId returns all of them (used by the analysis-history UI).
func (s *GormStore) GetAllArticleAnalyses(articleId, profileId string) ([]models.ArticleAnalysis, error) {
	var analyses []models.ArticleAnalysis

	query := s.db.Where("article_id = ?", articleId)
	if profileId != "" {
		query = query.Where("profile_id = ?", profileId)
	}
	if err := query.Order("created_at DESC").Find(&analyses).Error; err != nil {
		return nil, fmt.Errorf("failed to get all article analyses: %w", err)
	}

	return analyses, nil
}
