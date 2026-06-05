package manager

import (
	"fmt"
	"strings"
	"time"

	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/pkg/models"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/PuerkitoBio/goquery"
)

// defaultInspectHTMLLimit caps how many bytes of page HTML InspectArticle returns
// when the caller does not specify a limit, so a huge page can't bloat the response.
const defaultInspectHTMLLimit = 200_000

// InspectFeedURL fetches and parses a feed URL (read-only), returning a diagnosis
// plus sample article links and the detected title. It is the pre-registration
// counterpart to DiagnoseFeed, working on a raw URL rather than a stored feed.
func (m *FeedManager) InspectFeedURL(feedURL string, headers map[string]string, maxLinks int) scrapers.FeedInspection {
	return scrapers.InspectFeedURL(feedURL, headers, maxLinks)
}

// InspectArticle scrapes a single article URL in the requested mode and, when an
// article selector is supplied, returns the extracted content. It is read-only and
// stores nothing — it backs the feed-config builder's selector testing and mode
// probing. Modes, in escalating cost: "" / static, dynamic, full_browser.
func (m *FeedManager) InspectArticle(rawURL, mode string, headers map[string]string, sel *models.Selectors, htmlLimit int) models.ArticleInspection {
	start := time.Now()
	insp := models.ArticleInspection{ModeUsed: normalizeMode(mode)}

	dom, err := m.scrapeArticleDOM(rawURL, mode, headers)
	insp.DurationMs = time.Since(start).Milliseconds()
	if err != nil {
		insp.Error = err.Error()
		return insp
	}

	html, _ := dom.Html()
	insp.RawHTMLLen = len(html)
	if htmlLimit <= 0 {
		htmlLimit = defaultInspectHTMLLimit
	}
	insp.HTML = truncateRunes(html, htmlLimit)

	if sel != nil && sel.Article != "" {
		// Whether the article selector matched must be read before extraction, which
		// mutates the DOM (blacklist/cutoff removal).
		insp.SelectorMatched = dom.Find(sel.Article).Length() > 0

		extractor := scrapers.NewArticleExtractor(config.Config.DefaultSelectors)
		content, exErr := extractor.ExtractFromDOM(dom, rawURL, sel)
		if exErr != nil {
			insp.Error = fmt.Sprintf("extraction failed: %v", exErr)
			return insp
		}
		// ExtractFromDOM returns HTML when the selector matched but markdown on
		// fallback; normalize matched content to markdown so the sample is readable
		// and char counts are comparable across candidates.
		if insp.SelectorMatched {
			if md, cErr := htmltomarkdown.ConvertString(content); cErr == nil {
				content = md
			}
		}
		insp.Extracted = content
		insp.ExtractedLen = len([]rune(content))
	}

	return insp
}

// scrapeArticleDOM fetches an article page in the given mode and returns its DOM.
// It centralizes the static/dynamic/full_browser dispatch so both FetchFeed-style
// scraping and the inspect path share one definition of each mode.
func (m *FeedManager) scrapeArticleDOM(rawURL, mode string, headers map[string]string) (*goquery.Selection, error) {
	anon := scrapers.GetSharedAnonymizedScraper()

	switch normalizeMode(mode) {
	case "static":
		return anon.ScrapeContent(rawURL, headers)
	case "dynamic":
		return anon.ScrapeContentWithPlaywright(rawURL, headers)
	case "full_browser":
		if m.solimenAddr == "" {
			return nil, fmt.Errorf("full_browser mode requires a configured solimen address")
		}
		res, err := solimenScrape("inspect", m.solimenAddr, rawURL, models.HostTriggers{})
		if err != nil {
			return nil, err
		}
		if res.State == "failed" {
			return nil, fmt.Errorf("solimen reported a failed state for %s", rawURL)
		}
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(res.HTML))
		if err != nil {
			return nil, fmt.Errorf("failed to parse solimen HTML: %w", err)
		}
		return doc.Selection, nil
	default:
		return nil, fmt.Errorf("unknown scraping mode %q", mode)
	}
}

// normalizeMode maps the empty/default mode to its canonical "static" name.
func normalizeMode(mode string) string {
	if mode == "" {
		return "static"
	}
	return mode
}

// truncateRunes returns the first n runes of s, appending a notice when cut.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "\n… (truncated)"
}
