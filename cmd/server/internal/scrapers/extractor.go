package scrapers

import (
	"downlink/pkg/models"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
	log "github.com/sirupsen/logrus"
)

// ArticleExtractor implements the Scraper interface for article content extraction
type ArticleExtractor struct {
	configSelectors *models.Selectors // Default selectors from config
}

// NewArticleExtractor creates a new ArticleExtractor instance
func NewArticleExtractor(configSelectors *models.Selectors) *ArticleExtractor {
	extractor := &ArticleExtractor{
		configSelectors: configSelectors,
	}

	return extractor
}

func (ae *ArticleExtractor) ExtractFromDOM(dom *goquery.Selection, url string, feedSelectors *models.Selectors) (string, error) {
	var extractedContent string

	// Start from config defaults, then let per-feed selectors override individual fields.
	effectiveSelectors := &models.Selectors{}
	if ae.configSelectors != nil {
		effectiveSelectors.Article = ae.configSelectors.Article
		effectiveSelectors.Cutoff = ae.configSelectors.Cutoff
		effectiveSelectors.Blacklist = ae.configSelectors.Blacklist
	}
	if feedSelectors != nil {
		if feedSelectors.Article != "" {
			effectiveSelectors.Article = feedSelectors.Article
		}
		if feedSelectors.Cutoff != "" {
			effectiveSelectors.Cutoff = feedSelectors.Cutoff
		}
		if feedSelectors.Blacklist != "" {
			effectiveSelectors.Blacklist = feedSelectors.Blacklist
		}
	}

	// Remove unwanted elements
	if effectiveSelectors.Blacklist != "" {
		dom.Find(effectiveSelectors.Blacklist).Remove()
	}

	// Try to find content using selectors
	content := dom.Find(effectiveSelectors.Article).First()
	if content.Length() > 0 {
		log.WithFields(log.Fields{
			"url":      url,
			"selector": effectiveSelectors.Article,
		}).Info("Found content using article selector")

		// Apply cutoff if specified
		if effectiveSelectors.Cutoff != "" {
			content.Find(effectiveSelectors.Cutoff).NextAll().Remove()
		}

		htmlContent, htmlErr := content.Html()
		if htmlErr != nil {
			log.Printf("Failed to get HTML for %s: %v", url, htmlErr)
			return "error fetching content", htmlErr
		}
		extractedContent = htmlContent
	} else {
		log.WithFields(log.Fields{
			"url":      url,
			"selector": effectiveSelectors.Article,
		}).Info("No content found, falling back to default extraction")

		html, err := dom.Html()
		if err != nil {
			log.Printf("Failed to get HTML for %s: %v", url, err)
			return "error fetching content", err
		}

		markdown, err := htmltomarkdown.ConvertString(html)
		if err != nil {
			log.Printf("Failed to convert HTML to Markdown for %s: %v", url, err)
			return "", err
		}

		extractedContent = markdown
	}

	return extractedContent, nil
}

// GetSelectors returns the configured selectors for this extractor
func (ae *ArticleExtractor) GetSelectors() *models.Selectors {
	return ae.configSelectors
}

// SetSelectors updates the config selectors
func (ae *ArticleExtractor) SetSelectors(selectors *models.Selectors) {
	ae.configSelectors = selectors
}
