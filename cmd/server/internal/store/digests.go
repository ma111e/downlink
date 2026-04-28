package store

import (
	"downlink/pkg/models"
	"fmt"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *GormStore) StoreDigest(digest models.Digest) error {
	result := s.db.Save(&digest)
	if result.Error != nil {
		return fmt.Errorf("failed to store digest: %w", result.Error)
	}
	return nil
}

func (s *GormStore) GetDigest(id string) (models.Digest, error) {
	var digest models.Digest

	// Preload provider results
	result := s.db.Preload("Articles").First(&digest, "id = ?", id)
	if result.Error != nil {
		return digest, fmt.Errorf("failed to get digest: %w", result.Error)
	}

	// Load provider results
	var providerResults []models.DigestProviderResult
	if err := s.db.Where("digest_id = ?", id).Find(&providerResults).Error; err != nil {
		return digest, fmt.Errorf("failed to load provider results: %w", err)
	}
	digest.ProviderResults = providerResults

	// Load digest analyses with their associated ArticleAnalysis
	var digestAnalyses []models.DigestAnalysis
	if err := s.db.Preload("Analysis").Where("digest_id = ?", id).Find(&digestAnalyses).Error; err != nil {
		return digest, fmt.Errorf("failed to load digest analyses: %w", err)
	}
	digest.DigestAnalyses = digestAnalyses

	return digest, nil
}

func (s *GormStore) ListDigests(limit int) ([]models.Digest, error) {
	var digests []models.Digest

	// Apply limit and order by creation date
	query := s.db.Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}

	result := query.Find(&digests)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list digests: %w", result.Error)
	}

	if len(digests) == 0 {
		return digests, nil
	}

	// Collect all digest IDs for batch loading
	ids := make([]string, len(digests))
	for i, d := range digests {
		ids[i] = d.Id
	}

	// Batch load provider results
	var allProviderResults []models.DigestProviderResult
	if err := s.db.Where("digest_id IN ?", ids).Find(&allProviderResults).Error; err != nil {
		log.WithError(err).Warn("Failed to batch load provider results for digests")
	}

	// Batch load digest analyses with associated ArticleAnalysis
	var allDigestAnalyses []models.DigestAnalysis
	if err := s.db.Preload("Analysis").Where("digest_id IN ?", ids).Find(&allDigestAnalyses).Error; err != nil {
		log.WithError(err).Warn("Failed to batch load digest analyses for digests")
	}

	// Build lookup maps and assign to digests
	providerResultsByDigest := make(map[string][]models.DigestProviderResult, len(digests))
	for _, r := range allProviderResults {
		providerResultsByDigest[r.DigestId] = append(providerResultsByDigest[r.DigestId], r)
	}
	analysesByDigest := make(map[string][]models.DigestAnalysis, len(digests))
	for _, a := range allDigestAnalyses {
		analysesByDigest[a.DigestId] = append(analysesByDigest[a.DigestId], a)
	}

	for i := range digests {
		digests[i].ProviderResults = providerResultsByDigest[digests[i].Id]
		digests[i].DigestAnalyses = analysesByDigest[digests[i].Id]
	}

	return digests, nil
}

func (s *GormStore) StoreDigestArticle(digestId string, articleId string) error {
	// Check if the digest exists
	var digest models.Digest
	if err := s.db.First(&digest, "id = ?", digestId).Error; err != nil {
		return fmt.Errorf("digest not found: %w", err)
	}

	// Check if the article exists
	var article models.Article
	if err := s.db.First(&article, "id = ?", articleId).Error; err != nil {
		return fmt.Errorf("article not found: %w", err)
	}

	// Add the article to the digest
	if err := s.db.Model(&digest).Association("Articles").Append(&article); err != nil {
		return fmt.Errorf("failed to associate article with digest: %w", err)
	}

	// Update the article count
	if err := s.db.Model(&digest).Update("article_count", gorm.Expr("article_count + 1")).Error; err != nil {
		return fmt.Errorf("failed to update digest article count: %w", err)
	}

	return nil
}

func (s *GormStore) GetDigestArticles(digestId string) ([]models.Article, error) {
	var articles []models.Article

	// Query articles through the many-to-many relationship
	if err := s.db.Model(&models.Digest{Id: digestId}).Association("Articles").Find(&articles); err != nil {
		return nil, fmt.Errorf("failed to get digest articles: %w", err)
	}

	if len(articles) == 0 {
		return articles, nil
	}

	// Collect article IDs and reload with preloaded tags in a single query
	ids := make([]string, len(articles))
	for i, a := range articles {
		ids[i] = a.Id
	}
	if err := s.db.Preload("Tags").Where("id IN ?", ids).Find(&articles).Error; err != nil {
		log.WithError(err).Warn("Failed to preload tags for digest articles")
	}

	return articles, nil
}

// StoreDigestArticlesBatch associates a batch of articles with a digest in a single operation.
func (s *GormStore) StoreDigestArticlesBatch(digestId string, articleIds []string) error {
	if len(articleIds) == 0 {
		return nil
	}

	// Build junction rows for bulk insert
	type digestArticle struct {
		DigestId  string `gorm:"column:digest_id"`
		ArticleId string `gorm:"column:article_id"`
	}
	rows := make([]digestArticle, len(articleIds))
	for i, id := range articleIds {
		rows[i] = digestArticle{DigestId: digestId, ArticleId: id}
	}

	if err := s.db.Table("digest_articles").Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
		return fmt.Errorf("failed to batch associate articles with digest: %w", err)
	}

	// Update article_count in a single query
	count := len(articleIds)
	if err := s.db.Model(&models.Digest{}).Where("id = ?", digestId).
		Update("article_count", gorm.Expr("article_count + ?", count)).Error; err != nil {
		return fmt.Errorf("failed to update digest article count: %w", err)
	}

	return nil
}

// FindDigestsWithSameArticleSet returns all digests (including digestId itself)
// whose digest_articles set is exactly equal to digestId's set, ordered by
// created_at DESC. ProviderResults are populated so callers can label each
// sibling by its provider/model.
func (s *GormStore) FindDigestsWithSameArticleSet(digestId string) ([]models.Digest, error) {
	const equalSetQuery = `
SELECT digest_id FROM digest_articles
WHERE digest_id IN (
  SELECT digest_id FROM digest_articles
  WHERE article_id IN (SELECT article_id FROM digest_articles WHERE digest_id = ?)
  GROUP BY digest_id
  HAVING COUNT(*) = (SELECT COUNT(*) FROM digest_articles WHERE digest_id = ?)
)
GROUP BY digest_id
HAVING COUNT(*) = (SELECT COUNT(*) FROM digest_articles WHERE digest_id = ?)`

	var ids []string
	if err := s.db.Raw(equalSetQuery, digestId, digestId, digestId).Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("failed to find sibling digest ids: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var digests []models.Digest
	if err := s.db.Where("id IN ?", ids).Order("created_at DESC").Find(&digests).Error; err != nil {
		return nil, fmt.Errorf("failed to load sibling digests: %w", err)
	}

	var providerResults []models.DigestProviderResult
	if err := s.db.Where("digest_id IN ?", ids).Find(&providerResults).Error; err != nil {
		log.WithError(err).Warn("Failed to batch load provider results for sibling digests")
	}
	resultsByDigest := make(map[string][]models.DigestProviderResult, len(digests))
	for _, r := range providerResults {
		resultsByDigest[r.DigestId] = append(resultsByDigest[r.DigestId], r)
	}
	for i := range digests {
		digests[i].ProviderResults = resultsByDigest[digests[i].Id]
	}

	return digests, nil
}
