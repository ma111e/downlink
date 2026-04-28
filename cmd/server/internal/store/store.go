package store

import (
	"downlink/pkg/models"
	"time"
)

type Store interface {
	Close() error

	StoreFeed(feed models.Feed) error
	GetFeed(id string) (models.Feed, error)
	ListFeeds() ([]models.Feed, error)
	UpdateFeedLastFetch(id string, lastFetch time.Time) error
	DeleteFeed(id string) error // Method for deleting a feed

	StoreArticle(article models.Article) error
	GetArticle(id string) (models.Article, error)
	GetArticlesBatch(ids []string) ([]models.Article, error)
	ListArticles(filter models.ArticleFilter) ([]models.Article, error)
	GetArticleCounts(filter models.ArticleFilter) (models.ArticleCounts, error)
	UpdateArticle(id string, update models.ArticleUpdate) error
	MarkFeedArticlesAsRead(feedId string) error
	DeleteFeedArticles(feedId string) error

	StoreDigest(digest models.Digest) error
	GetDigest(id string) (models.Digest, error)
	ListDigests(limit int) ([]models.Digest, error)
	StoreDigestArticle(digestId string, articleId string) error
	StoreDigestArticlesBatch(digestId string, articleIds []string) error
	GetDigestArticles(digestId string) ([]models.Article, error)
	FindDigestsWithSameArticleSet(digestId string) ([]models.Digest, error)

	StoreDigestProviderResult(result models.DigestProviderResult) error
	GetDigestProviderResults(digestId string) ([]models.DigestProviderResult, error)
	GetDigestProviderResult(id string) (models.DigestProviderResult, error)

	StoreDigestAnalysis(entry models.DigestAnalysis) error
	StoreDigestAnalysesBatch(entries []models.DigestAnalysis) error
	GetDigestAnalyses(digestId string) ([]models.DigestAnalysis, error)

	GetCategories() ([]models.Category, error)
	SaveCategory(category models.Category) error
	GetOrCreateCategory(name string) (*models.Category, error)

	SaveArticleAnalysis(analysis *models.ArticleAnalysis) error
	GetArticleAnalysis(articleId string) (*models.ArticleAnalysis, error)
	GetArticleAnalysesBatch(articleIds []string) (map[string]*models.ArticleAnalysis, error)
	GetAllArticleAnalyses(articleId string) ([]models.ArticleAnalysis, error)

	ListFeedGroups() ([]models.FeedGroup, error)
	GetFeedGroup(id string) (models.FeedGroup, error)
	StoreFeedGroup(group models.FeedGroup) error
	DeleteFeedGroup(id string) error
}
