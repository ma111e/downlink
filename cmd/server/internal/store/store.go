package store

import (
	"github.com/ma111e/downlink/pkg/models"
	"time"
)

type Store interface {
	Close() error

	StoreFeed(feed models.Feed) error
	GetFeed(id string) (models.Feed, error)
	ListFeeds() ([]models.Feed, error)
	UpdateFeedLastFetch(id string, lastFetch time.Time) error
	DeleteFeed(id string) error // Method for deleting a feed

	SetFeedTopics(feedId string, topics []string) error
	ListFeedTopics(feedId string) ([]string, error)
	ListAllTopics() ([]string, error)
	FeedIDsByTopics(topics []string) ([]string, error)
	ListEnabledFeedIDs() ([]string, error)

	StoreArticle(article models.Article) error
	GetArticle(id string) (models.Article, error)
	GetArticlesBatch(ids []string) ([]models.Article, error)
	ListArticles(filter models.ArticleFilter) ([]models.Article, error)
	GetArticleCounts(filter models.ArticleFilter) (models.ArticleCounts, error)
	UpdateArticle(id string, update models.ArticleUpdate) error
	MarkFeedArticlesAsRead(feedId string) error
	DeleteFeedArticles(feedId string) error
	DeleteUnusedTags() error

	StoreDigest(digest models.Digest) error
	GetDigest(id string) (models.Digest, error)
	ListDigests(limit int, full bool) ([]models.Digest, error)
	ListDigestsByProfile(profileId string, limit int, full bool) ([]models.Digest, error)
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

	UpsertGlossaryEntry(entry *models.GlossaryEntry) error
	GetGlossaryEntriesByKeys(keys []string) (map[string]*models.GlossaryEntry, error)
	ListGlossaryEntries(limit int) ([]models.GlossaryEntry, error)
	StoreDigestGlossaryBatch(rows []models.DigestGlossary) error
	GetDigestGlossary(digestId string) ([]models.DigestGlossary, error)
	SetGlossaryManualOverride(key, curatedDef string) error

	GetCategories() ([]models.Category, error)
	SaveCategory(category models.Category) error
	GetOrCreateCategory(name string) (*models.Category, error)

	SaveArticleAnalysis(analysis *models.ArticleAnalysis) error
	UpdateArticleAnalysisGlossaryTerms(id, glossaryTermsJson string) error
	GetArticleAnalysis(articleId, profileId string) (*models.ArticleAnalysis, error)
	GetArticleAnalysesBatch(articleIds []string, profileId string) (map[string]*models.ArticleAnalysis, error)
	GetAllArticleAnalyses(articleId, profileId string) ([]models.ArticleAnalysis, error)
	PruneAnalyses(keep int) error

	ListFeedGroups() ([]models.FeedGroup, error)
	GetFeedGroup(id string) (models.FeedGroup, error)
	StoreFeedGroup(group models.FeedGroup) error
	DeleteFeedGroup(id string) error

	ListProfiles() ([]models.Profile, error)
	GetProfile(id string) (models.Profile, error)
	StoreProfile(profile models.Profile) error
	DeleteProfile(id string) error
	SetProfileFeeds(profileId string, feedIds []string) error
	ListProfileFeeds(profileId string) ([]models.Feed, error)

	ListLLMRunSummaries(limit int, profileID string) ([]LLMRunSummary, error)
	GetLLMRun(id string) (models.LLMRun, error)
	ListLLMCallsForRun(runID string) ([]LLMCallView, error)

	StartFeedRefreshRun(id, trigger string, startedAt time.Time) error
	RecordFeedRefresh(in FeedRefreshInput) error
	FinishFeedRefreshRun(id string, finishedAt time.Time) error
	ListFeedRefreshRunSummaries(limit int) ([]FeedRefreshRunSummary, error)
	GetFeedRefreshRun(id string) (models.FeedRefreshRun, error)
	ListFeedRefreshResultsForRun(runID string) ([]FeedRefreshResultView, error)
	PruneFeedRefreshRuns(keep int) error
}
