package store

import (
	"downlink/pkg/models"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

func (s *GormStore) StoreArticle(article models.Article) error {
	// Verify the feed exists first
	var feedCount int64
	if err := s.db.Model(&models.Feed{}).Where("id = ?", article.FeedId).Count(&feedCount).Error; err != nil {
		return fmt.Errorf("failed to check if feed exists: %w", err)
	}

	// Nullable fields MUST be set as nil instead of "", which is interpreted as a value
	if article.CategoryName != nil && *article.CategoryName == "" {
		article.CategoryName = nil
	}

	// Start a transaction
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// Handle tags separately
	originalTags := article.Tags
	article.Tags = nil // Clear tags to avoid FK issues in initial save

	// Save the article without tags first
	if err := tx.Save(&article).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to store article: %w", err)
	}

	// Handle tags if there are any
	if len(originalTags) > 0 {
		// Clear existing article_tags associations
		if err := tx.Exec("DELETE FROM article_tags WHERE article_id = ?", article.Id).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to clear existing article tags: %w", err)
		}

		// Upsert all tags in a single statement using INSERT OR IGNORE
		for _, tag := range originalTags {
			if err := tx.Exec("INSERT OR IGNORE INTO tags (id, name) VALUES (?, ?)", tag.Id, tag.Name).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to upsert tag: %w", err)
			}
		}

		// Batch insert all article_tags associations
		placeholders := make([]string, len(originalTags))
		args := make([]interface{}, 0, len(originalTags)*2)
		for i, tag := range originalTags {
			placeholders[i] = "(?, ?)"
			args = append(args, article.Id, tag.Id)
		}
		sql := "INSERT OR IGNORE INTO article_tags (article_id, tag_id) VALUES " + strings.Join(placeholders, ", ")
		if err := tx.Exec(sql, args...).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to batch associate tags with article: %w", err)
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.WithFields(log.Fields{
		"id":        article.Id,
		"feed_id":   article.FeedId,
		"title":     article.Title,
		"link":      article.Link,
		"published": article.PublishedAt,
	}).Info("Article stored successfully")

	return nil
}

func (s *GormStore) applyArticleFilters(query *gorm.DB, filter models.ArticleFilter) *gorm.DB {
	if filter.UnreadOnly {
		query = query.Where("read = ?", false)
	}

	if filter.BookmarkedOnly {
		query = query.Where("bookmarked = ?", true)
	}

	if filter.CategoryName != "" {
		query = query.Where("category_name = ?", filter.CategoryName)
	}

	if filter.StartDate != nil {
		query = query.Where("published_at >= ?", filter.StartDate)
	}

	if filter.EndDate != nil {
		query = query.Where("published_at <= ?", filter.EndDate)
	}

	if filter.FeedId != "" {
		query = query.Where("feed_id = ?", filter.FeedId)
	}

	if filter.ExcludeDigested {
		query = query.Where("id NOT IN (SELECT article_id FROM digest_articles)")
	}

	if trimmed := strings.TrimSpace(filter.Query); trimmed != "" {
		query = query.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(trimmed)+"%")
	}

	return query
}

func normalizeArticleListLimit(limit int) int {
	switch {
	case limit <= 0:
		return 30
	case limit > 50:
		return 50
	default:
		return limit
	}
}

func (s *GormStore) GetArticle(id string) (models.Article, error) {
	var article models.Article

	// Preload tags and category
	result := s.db.Preload("Tags").Preload("Category").First(&article, "id = ?", id)
	if result.Error != nil {
		return article, fmt.Errorf("failed to get article: %w", result.Error)
	}

	// Load related articles
	// if err := s.loadRelatedArticles(&article); err != nil {
	// 	log.WithError(err).Warn("Failed to load related articles")
	// 	// Continue without related articles if there's an error
	// }

	return article, nil
}

func (s *GormStore) loadRelatedArticles(article *models.Article) error {
	var relatedEntries []models.RelatedArticle
	if err := s.db.Where("article_id = ?", article.Id).Find(&relatedEntries).Error; err != nil {
		return err
	}

	article.RelatedArticles = relatedEntries
	return nil
}

func (s *GormStore) ListArticles(filter models.ArticleFilter) ([]models.Article, error) {
	var articles []models.Article

	// Start with a base query.
	// Skip the (potentially huge) `content` column — list views only need metadata.
	// The full content is fetched on demand via GetArticle.
	listColumns := []string{
		"id", "feed_id", "title", "link",
		"published_at", "fetched_at",
		"read", "category_name", "hero_image", "bookmarked",
	}
	query := s.db.Model(&models.Article{}).Select(listColumns).Preload("Tags")

	query = s.applyArticleFilters(query, filter)

	if filter.TagId != "" {
		// This requires a join on the many-to-many relationship
		query = query.Joins("JOIN article_tags ON article_tags.article_id = articles.id").
			Where("article_tags.tag_id = ?", filter.TagId)
	}

	if filter.RelatedToId != "" {
		// Articles related to the specified article
		query = query.Joins("JOIN related_articles ON related_articles.related_article_id = articles.id").
			Where("related_articles.article_id = ?", filter.RelatedToId)
	}

	// Order by published date descending
	query = query.Order("published_at DESC")

	// Add a reasonable limit and optional offset for pagination
	query = query.Limit(normalizeArticleListLimit(filter.Limit))
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Execute the query
	result := query.Find(&articles)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to list articles: %w", result.Error)
	}

	// Populate LatestImportanceScore in one batch query (one extra DB round-trip
	// instead of one per article; avoids N+1 from the frontend side).
	if len(articles) > 0 {
		ids := make([]string, len(articles))
		for i := range articles {
			ids[i] = articles[i].Id
		}
		latestScores, err := s.latestImportanceScores(ids)
		if err != nil {
			log.WithError(err).Warn("Failed to load latest importance scores; continuing without")
		} else {
			for i := range articles {
				if score, ok := latestScores[articles[i].Id]; ok {
					s := score
					articles[i].LatestImportanceScore = &s
				}
			}
		}
	}

	return articles, nil
}

func (s *GormStore) GetArticleCounts(filter models.ArticleFilter) (models.ArticleCounts, error) {
	counts := models.ArticleCounts{
		UnreadByFeed: make(map[string]int64),
	}

	baseQuery := s.applyArticleFilters(s.db.Model(&models.Article{}), filter)

	if err := baseQuery.Where("read = ?", false).Count(&counts.AllUnreadCount).Error; err != nil {
		return counts, fmt.Errorf("failed to count unread articles: %w", err)
	}

	if err := s.applyArticleFilters(s.db.Model(&models.Article{}), filter).
		Where("bookmarked = ?", true).
		Count(&counts.BookmarkedCount).Error; err != nil {
		return counts, fmt.Errorf("failed to count bookmarked articles: %w", err)
	}

	type unreadRow struct {
		FeedId string
		Count  int64
	}
	var unreadRows []unreadRow
	if err := s.applyArticleFilters(s.db.Model(&models.Article{}), filter).
		Select("feed_id, COUNT(*) AS count").
		Where("read = ?", false).
		Group("feed_id").
		Scan(&unreadRows).Error; err != nil {
		return counts, fmt.Errorf("failed to count unread articles by feed: %w", err)
	}

	for _, row := range unreadRows {
		counts.UnreadByFeed[row.FeedId] = row.Count
	}

	return counts, nil
}

// latestImportanceScores returns a map of articleId -> latest importance_score
// for the given article ids, using a single SQL query.
func (s *GormStore) latestImportanceScores(articleIds []string) (map[string]int32, error) {
	if len(articleIds) == 0 {
		return map[string]int32{}, nil
	}
	type row struct {
		ArticleId       string
		ImportanceScore int32
	}
	var rows []row
	// Inner query selects the most recent created_at per article, then we
	// join back to read its importance_score.
	sql := `
		SELECT a.article_id AS article_id, a.importance_score AS importance_score
		FROM article_analyses a
		INNER JOIN (
			SELECT article_id, MAX(created_at) AS max_created
			FROM article_analyses
			WHERE article_id IN ?
			GROUP BY article_id
		) latest ON latest.article_id = a.article_id AND latest.max_created = a.created_at
	`
	if err := s.db.Raw(sql, articleIds).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int32, len(rows))
	for _, r := range rows {
		out[r.ArticleId] = r.ImportanceScore
	}
	return out, nil
}

func (s *GormStore) UpdateArticle(id string, update models.ArticleUpdate) error {
	tx := s.db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	// First get the current article to update
	var article models.Article
	if err := tx.First(&article, "id = ?", id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to get article for update: %w", err)
	}

	// Apply updates
	updates := make(map[string]interface{})

	if update.Read != nil {
		updates["read"] = *update.Read
	}

	if update.CategoryName != nil {
		updates["category_name"] = *update.CategoryName
	}

	if update.HeroImage != nil {
		updates["hero_image"] = *update.HeroImage
	}

	if update.Bookmarked != nil {
		updates["bookmarked"] = *update.Bookmarked
	}

	// Apply basic updates
	if len(updates) > 0 {
		if err := tx.Model(&article).Updates(updates).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to update article fields: %w", err)
		}
	}

	// Update tags if provided
	if update.TagIds != nil && len(*update.TagIds) > 0 {
		// First clear existing tags
		if err := tx.Model(&article).Association("Tags").Clear(); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to clear existing tags: %w", err)
		}

		// Add new tags
		for _, tagId := range *update.TagIds {
			var tag models.Tag
			if err := tx.FirstOrCreate(&tag, models.Tag{Id: tagId, Name: tagId}).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to get/create tag: %w", err)
			}

			if err := tx.Model(&article).Association("Tags").Append(&tag); err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to associate tag: %w", err)
			}
		}
	}

	// Update related articles if provided
	if update.RelatedArticles != nil && len(*update.RelatedArticles) > 0 {
		// First delete existing related articles
		if err := tx.Where("article_id = ?", id).Delete(&models.RelatedArticle{}).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to delete existing related articles: %w", err)
		}

		// Add new related articles
		for _, relatedArticle := range *update.RelatedArticles {
			relatedArticle.ArticleId = id
			if err := tx.Create(&relatedArticle).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to create related article: %w", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (s *GormStore) MarkFeedArticlesAsRead(feedId string) error {
	if err := s.db.Model(&models.Article{}).
		Where("feed_id = ? AND read = ?", feedId, false).
		Update("read", true).Error; err != nil {
		return fmt.Errorf("failed to mark feed articles as read: %w", err)
	}

	return nil
}

func (s *GormStore) GetArticlesBatch(ids []string) ([]models.Article, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var articles []models.Article
	if err := s.db.Preload("Tags").Preload("Category").Where("id IN ?", ids).Find(&articles).Error; err != nil {
		return nil, fmt.Errorf("failed to get articles batch: %w", err)
	}
	return articles, nil
}

func (s *GormStore) DeleteFeedArticles(feedId string) error {
	result := s.db.Where("feed_id = ?", feedId).Delete(&models.Article{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete feed articles: %w", result.Error)
	}
	return nil
}
