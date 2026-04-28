package scrapers

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"downlink/pkg/models"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"

	log "github.com/sirupsen/logrus"
)

// RSSFeedScraper implements the Scraper interface for RSS feeds
type RSSFeedScraper struct {
	parser          *gofeed.Parser
	configSelectors *models.Selectors // Default selectors from config
}

// NewRSSFeedScraper creates a new RSSFeedScraper instance
func NewRSSFeedScraper(configSelectors *models.Selectors) *RSSFeedScraper {
	return &RSSFeedScraper{
		parser:          gofeed.NewParser(),
		configSelectors: configSelectors,
	}
}

// Fetch fetches and parses an RSS feed
func (s *RSSFeedScraper) Fetch(url string) ([]models.FeedItem, error) {
	log.WithField("url", url).Debug("Fetching RSS feed")

	// Parse the feed
	feed, err := s.parser.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("failed to parse feed: %w", err)
	}

	// Convert feed items to FeedItems
	items := make([]models.FeedItem, 0, len(feed.Items))
	for _, item := range feed.Items {
		// Generate a unique Id for the item
		itemId := ""
		if item.GUID != "" {
			itemId = item.GUID
		} else if item.Link != "" {
			hash := md5.Sum([]byte(item.Link))
			itemId = fmt.Sprintf("%x", hash)
		} else {
			hash := md5.Sum([]byte(item.Title + item.Description))
			itemId = fmt.Sprintf("%x", hash)
		}

		// Create FeedItem
		feedItem := models.FeedItem{
			Id:       itemId,
			Title:    item.Title,
			Link:     item.Link,
			Tags:     []string{}, // Placeholder for tagging
			Category: "",         // Placeholder for categorization
		}

		// Handle content
		if item.Content != "" {
			feedItem.Content = item.Content
		} else if item.Description != "" {
			feedItem.Content = item.Description
		}

		// Extract hero image from content or enclosures
		heroImage := extractHeroImage(item)
		if heroImage != "" {
			feedItem.HeroImage = heroImage
		}

		// Handle published date
		if item.PublishedParsed != nil {
			feedItem.PublishedAt = *item.PublishedParsed
		} else if item.UpdatedParsed != nil {
			feedItem.PublishedAt = *item.UpdatedParsed
		} else {
			feedItem.PublishedAt = time.Now()
		}

		items = append(items, feedItem)
	}

	log.WithFields(log.Fields{
		"url":   url,
		"items": len(items),
	}).Debug("RSS feed fetched successfully")

	return items, nil
}

func (s *RSSFeedScraper) ScrapeContent(url string, params map[string]any) (string, error) {
	domain := strings.Split(strings.Split(url, "://")[1], "/")[0]

	// Use the shared scraper defined in the scraper package
	anonymizedScraper := GetSharedAnonymizedScraper(domain)

	var dom *goquery.Selection
	var err error

	scrapingMode, _ := params["scraping"].(string)
	if scrapingMode == "dynamic" {
		log.WithFields(log.Fields{
			"url":    url,
			"method": "dynamic",
		}).Debug("Using dynamic scraping (Playwright)")
		dom, err = anonymizedScraper.ScrapeContentWithPlaywright(url)
	} else {
		log.WithFields(log.Fields{
			"url":    url,
			"method": "static",
		}).Debug("Using static scraping (HTTP)")
		dom, err = anonymizedScraper.ScrapeContent(url)
	}

	if err != nil {
		log.WithFields(log.Fields{
			"url":      url,
			"scraping": scrapingMode,
			"err":      err,
		}).Error("Failed to scrape content")
		return "", err
	}

	log.WithFields(log.Fields{
		"url": url,
	}).Info("Extracting article")

	// Convert params["selectors"] from map[string]interface{} to models.Selectors
	var selectors *models.Selectors
	if selectorsData, ok := params["selectors"]; ok && selectorsData != nil {
		// Marshal the map to JSON and then unmarshal to the struct
		selectorsBytes, err := json.Marshal(selectorsData)
		if err != nil {
			return "", fmt.Errorf("failed to marshal selectors: %w", err)
		}

		selectors = &models.Selectors{}
		if err := json.Unmarshal(selectorsBytes, selectors); err != nil {
			return "", fmt.Errorf("failed to unmarshal selectors: %w", err)
		}
	}

	extractor := NewArticleExtractor(s.configSelectors)
	extracted, err := extractor.ExtractFromDOM(dom, url, selectors)
	if err != nil {
		log.WithFields(log.Fields{
			"url": url,
			"err": err,
		}).Error("Failed to extract article")
		return "", err
	}

	return extracted, nil
}
