package scrapers

import (
	"bytes"
	"crypto/md5"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/models"

	"github.com/PuerkitoBio/goquery"
	log "github.com/sirupsen/logrus"
)

// HTMLLinkScraper ingests a blog index page that lists links to posts instead of
// an RSS/Atom feed. Fetch turns the matched anchors into feed items (no content);
// ScrapeContent then fetches and extracts each linked article exactly like the
// RSS scraper does.
type HTMLLinkScraper struct {
	configSelectors *models.Selectors
}

// NewHTMLLinkScraper creates an HTMLLinkScraper using the given default selectors
// for article extraction.
func NewHTMLLinkScraper(configSelectors *models.Selectors) *HTMLLinkScraper {
	return &HTMLLinkScraper{configSelectors: configSelectors}
}

// Fetch GETs the index page and returns one FeedItem per matched post link.
// Required param: links_selector (CSS selector scoping the post anchors).
// Optional param: url_filter (substring an href must contain to be kept).
func (s *HTMLLinkScraper) Fetch(feedURL string, params map[string]any) ([]models.FeedItem, *RawResponse, error) {
	linksSelector, _ := params["links_selector"].(string)
	if strings.TrimSpace(linksSelector) == "" {
		return nil, nil, fmt.Errorf("html scraper requires a 'links_selector' option")
	}
	urlFilter, _ := params["url_filter"].(string)

	log.WithField("url", feedURL).Debug("Fetching HTML link list")

	raw, err := FetchRaw(feedURL, HeadersFromParams(params))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch index page: %w", err)
	}

	// Resolve relative links against the final URL (after redirects) when available.
	base := raw.FinalURL
	if base == "" {
		base = feedURL
	}

	items, err := parseLinkList(raw.Body, base, linksSelector, urlFilter)
	if err != nil {
		return nil, &raw, err
	}

	log.WithFields(log.Fields{
		"url":   feedURL,
		"items": len(items),
	}).Debug("HTML link list fetched successfully")

	return items, &raw, nil
}

// ScrapeContent fetches and extracts a single linked article, reusing the shared
// static/dynamic scrape-and-extract pipeline.
func (s *HTMLLinkScraper) ScrapeContent(articleURL string, params map[string]any) (string, error) {
	return scrapeAndExtract(articleURL, params, s.configSelectors)
}

// parseLinkList parses index-page HTML and returns a FeedItem per anchor matched
// by linksSelector. Relative hrefs are resolved against baseURL; when urlFilter is
// non-empty, only hrefs containing it are kept. Empty and duplicate hrefs are
// dropped. Item ids are md5(resolved href) so re-fetches dedupe deterministically.
func parseLinkList(body []byte, baseURL, linksSelector, urlFilter string) ([]models.FeedItem, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse index page HTML: %w", err)
	}

	base, baseErr := url.Parse(baseURL)

	var items []models.FeedItem
	seen := make(map[string]struct{})

	doc.Find(linksSelector).Each(func(_ int, a *goquery.Selection) {
		href, ok := a.Attr("href")
		if !ok {
			return
		}
		href = strings.TrimSpace(href)
		if href == "" {
			return
		}

		// Resolve relative hrefs against the page URL.
		resolved := href
		if baseErr == nil {
			if ref, perr := url.Parse(href); perr == nil {
				resolved = base.ResolveReference(ref).String()
			}
		}

		if urlFilter != "" && !strings.Contains(resolved, urlFilter) {
			return
		}
		if _, dup := seen[resolved]; dup {
			return
		}
		seen[resolved] = struct{}{}

		title := strings.TrimSpace(a.Text())
		if title == "" {
			title = resolved
		}

		hash := md5.Sum([]byte(resolved))
		items = append(items, models.FeedItem{
			Id:          fmt.Sprintf("%x", hash),
			Title:       title,
			Link:        resolved,
			Content:     "", // empty content forces the manager to scrape the article page
			Tags:        []string{},
			PublishedAt: time.Now(),
		})
	})

	return items, nil
}
