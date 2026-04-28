package manager

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"downlink/cmd/server/internal/config"
	"downlink/cmd/server/internal/scrapers"
	"downlink/pkg/models"

	"github.com/PuerkitoBio/goquery"
	log "github.com/sirupsen/logrus"
)

// ListFeeds returns all registered feeds
func (m *FeedManager) ListFeeds() ([]models.Feed, error) {
	return m.store.ListFeeds()
}

// GetFeed returns a feed by Id
func (m *FeedManager) GetFeed(id string) (models.Feed, error) {
	return m.store.GetFeed(id)
}

// FetchFeed fetches a feed and stores its articles
// Optional time window filtering: from and to can be nil to disable filtering
// If provided, only articles with PublishedAt between from and to (inclusive) will be stored
// If overwrite is true, existing articles will be overwritten instead of skipped
// If restore is true, only existing articles with no content will be overwritten
func (m *FeedManager) FetchFeed(feed models.Feed, from *time.Time, to *time.Time, overwrite bool, restore bool) (models.FetchResult, error) {
	result := models.FetchResult{}

	// Validate time window parameters
	if from != nil && to != nil {
		if to.Before(*from) {
			return result, fmt.Errorf("invalid time window: 'to' (%v) cannot be before 'from' (%v)", to, from)
		}
	}

	// Get scraper
	scraper, err := m.GetScraper(feed.Type)
	if err != nil {
		return result, err
	}

	// Fetch feed
	logFields := log.Fields{
		"id":   feed.Id,
		"url":  feed.URL,
		"type": feed.Type,
	}
	if from != nil {
		logFields["from"] = from.Format(time.RFC3339)
	}
	if to != nil {
		logFields["to"] = to.Format(time.RFC3339)
	}
	log.WithFields(logFields).Info("Fetching feed")

	// Fetch items
	items, err := scraper.Fetch(feed.URL)
	if err != nil {
		return result, fmt.Errorf("failed to fetch feed: %w", err)
	}
	result.TotalFetched = len(items)

	// Store items
	for _, item := range items {
		// Apply time window filter if specified
		if from != nil && item.PublishedAt.Before(*from) {
			log.WithFields(log.Fields{
				"title":     item.Title,
				"published": item.PublishedAt.Format(time.RFC3339),
				"from":      from.Format(time.RFC3339),
			}).Debug("Skipping article: published before time window")
			result.Skipped++
			continue
		}
		if to != nil && item.PublishedAt.After(*to) {
			log.WithFields(log.Fields{
				"title":     item.Title,
				"published": item.PublishedAt.Format(time.RFC3339),
				"to":        to.Format(time.RFC3339),
			}).Debug("Skipping article: published after time window")
			result.Skipped++
			continue
		}
		// Generate article Id
		articleId := generateArticleId(feed.Id, item.Id, item.Title)

		// Check if article already exists
		if !overwrite {
			existing, err := m.store.GetArticle(articleId)
			if err == nil {
				// restore mode: only overwrite if the existing article has no content
				if restore && existing.Content == "" {
					log.WithField("articleId", articleId).Debug("Article exists with no content, restoring")
				} else {
					log.WithField("articleId", articleId).Debug("Article already exists, skipping update")
					result.Skipped++
					continue
				}
			}
		}

		// Convert string tags to Tag objects
		var tags []models.Tag
		for _, tagStr := range item.Tags {
			tags = append(tags, models.Tag{
				Id:   tagStr,
				Name: tagStr,
			})
		}

		read := false

		// Create new article
		article := models.Article{
			Id:           articleId,
			FeedId:       feed.Id,
			Title:        item.Title,
			Content:      item.Content,
			Link:         item.Link,
			PublishedAt:  item.PublishedAt,
			FetchedAt:    time.Now(),
			Read:         &read, // New articles are unread
			Tags:         tags,
			CategoryName: &item.Category,
			HeroImage:    item.HeroImage,
		}

		// run scraper.ScrapeContent if content is <1000 chars as we guess it's not the full article
		scrapeFailed := false
		if len(article.Content) < 1000 {
			scrapingMode, _ := feed.Scraper["scraping"].(string)

			log.WithFields(log.Fields{
				"url":        article.Link,
				"scraping":   scrapingMode,
				"article_id": article.Id,
			}).Debug("Scraping article content")

			switch scrapingMode {
			case "full_browser":
				if m.solimenAddr == "" {
					log.WithField("article", article.Id).Error("full_browser scraping requested but solimen address is not configured")
					scrapeFailed = true
				} else {
					var triggers models.HostTriggers
					if t, ok := feed.Scraper["triggers"]; ok && t != nil {
						if b, merr := json.Marshal(t); merr == nil {
							json.Unmarshal(b, &triggers)
						}
					}

					scrapeResult, serr := solimenScrape(m.solimenAddr, article.Link, triggers)
					if serr != nil {
						log.WithError(serr).WithField("article", article.Id).Error("Failed to scrape article content via solimen")
						result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, serr))
						scrapeFailed = true
					} else if len(triggers.Failed) > 0 && scrapeResult.State == "failed" {
						serr = fmt.Errorf("page reported failed state")
						log.WithError(serr).WithField("article", article.Id).Error("Failed to scrape article content via solimen")
						result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, serr))
						scrapeFailed = true
					} else {
						var feedSelectors *models.Selectors
						if selData, ok := feed.Scraper["selectors"]; ok && selData != nil {
							if b, merr := json.Marshal(selData); merr == nil {
								feedSelectors = &models.Selectors{}
								if merr = json.Unmarshal(b, feedSelectors); merr != nil {
									feedSelectors = nil
								}
							}
						}
						doc, perr := goquery.NewDocumentFromReader(strings.NewReader(scrapeResult.HTML))
						if perr != nil {
							log.WithError(perr).WithField("article", article.Id).Error("Failed to parse DOM from solimen")
							result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, perr))
							scrapeFailed = true
						} else {
							extractor := scrapers.NewArticleExtractor(config.Config.DefaultSelectors)
							article.Content, err = extractor.ExtractFromDOM(doc.Selection, article.Link, feedSelectors)
							if err != nil {
								log.WithError(err).WithField("article", article.Id).Error("Failed to extract article content from solimen DOM")
								result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, err))
								scrapeFailed = true
							}
						}
					}
				}
			default:
				article.Content, err = scraper.ScrapeContent(article.Link, feed.Scraper)
				if err != nil {
					log.WithError(err).WithField("article", article.Id).Error("Failed to scrape article content")
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, err))
					scrapeFailed = true
				}
			}
		}

		// Skip storing if scraping was attempted but failed
		if scrapeFailed {
			log.WithField("article", article.Id).Debug("Skipping article storage due to failed scrape")
			result.Skipped++
			continue
		}

		// Check if content is valid UTF-8
		if !utf8.ValidString(article.Content) {
			log.WithField("article", article.Id).Error("Article content is not valid UTF-8, skipping")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: invalid UTF-8 content", item.Title))
			continue
		}

		if err := m.store.StoreArticle(article); err != nil {
			log.WithError(err).WithField("article", article.Id).Error("Failed to store article")
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", item.Title, err))
		} else {
			log.WithField("articleId", articleId).Debug("New article stored successfully")
			result.Stored++
		}
	}

	// Update feed last fetch time
	if err := m.store.UpdateFeedLastFetch(feed.Id, time.Now()); err != nil {
		log.WithError(err).WithField("feed", feed.Id).Error("Failed to update feed last fetch time")
	}

	log.WithFields(log.Fields{
		"id":      feed.Id,
		"items":   result.TotalFetched,
		"stored":  result.Stored,
		"skipped": result.Skipped,
		"errors":  len(result.Errors),
	}).Info("Feed fetched successfully")

	return result, nil
}

// RemoveFeed removes a feed and its data from the system
func (m *FeedManager) RemoveFeed(feedId string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if feed exists
	feed, err := m.store.GetFeed(feedId)
	if err != nil {
		return fmt.Errorf("feed not found: %w", err)
	}

	log.WithFields(log.Fields{
		"feedId": feedId,
		"title":  feed.Title,
		"url":    feed.URL,
	}).Info("Removing feed")

	// First delete all articles associated with this feed
	if err := m.store.DeleteFeedArticles(feedId); err != nil {
		return fmt.Errorf("failed to delete feed articles: %w", err)
	}

	// Then delete the feed itself
	if err := m.store.DeleteFeed(feedId); err != nil {
		return fmt.Errorf("failed to delete feed: %w", err)
	}

	log.WithField("feedId", feedId).Info("Feed removed successfully")
	return nil
}

// RefreshFeed refreshes a feed by Id
func (m *FeedManager) RefreshFeed(id string) (models.FetchResult, error) {
	feed, err := m.GetFeed(id)
	if err != nil {
		return models.FetchResult{}, err
	}

	return m.FetchFeed(feed, nil, nil, false, false)
}

// RefreshFeedWithTimeWindow refreshes a feed by Id with optional time window filtering
// from and to can be nil to disable filtering
func (m *FeedManager) RefreshFeedWithTimeWindow(id string, from *time.Time, to *time.Time, overwrite bool, restore bool) (models.FetchResult, error) {
	feed, err := m.GetFeed(id)
	if err != nil {
		return models.FetchResult{}, err
	}

	return m.FetchFeed(feed, from, to, overwrite, restore)
}

// UpdateFeedEnabled updates the enabled status of a feed
func (m *FeedManager) UpdateFeedEnabled(id string, enabled bool) error {
	feed, err := m.GetFeed(id)
	if err != nil {
		return err
	}

	feed.Enabled = &enabled
	return m.store.StoreFeed(feed)
}

// RefreshAllFeeds refreshes all enabled feeds
func (m *FeedManager) RefreshAllFeeds(wg *sync.WaitGroup) {
	feeds, err := m.ListFeeds()
	if err != nil {
		log.WithError(err).Error("Failed to list feeds")
		return
	}

	// Filter out disabled feeds
	enabledFeeds := []models.Feed{}
	for _, feed := range feeds {
		if feed.Enabled != nil && *feed.Enabled {
			enabledFeeds = append(enabledFeeds, feed)
		}
	}

	log.WithFields(log.Fields{
		"total":   len(feeds),
		"enabled": len(enabledFeeds),
	}).Info("Refreshing all enabled feeds")

	resultCh := make(chan models.FeedResult, len(enabledFeeds))

	// Create a worker pool
	for _, feed := range enabledFeeds {
		go func(feed models.Feed) {
			fetchResult, err := m.FetchFeed(feed, nil, nil, false, false)
			resultCh <- models.FeedResult{
				Feed:        feed,
				Error:       err,
				FetchResult: fetchResult,
			}
		}(feed)
	}

	// Process results in a separate goroutine
	go func() {
		for range enabledFeeds {
			result := <-resultCh
			if result.Error != nil {
				log.WithFields(log.Fields{
					"feed": result.Feed.Id,
					"err":  result.Error,
				}).Error("Failed to refresh feed")
			}
		}
		close(resultCh)
	}()
}
