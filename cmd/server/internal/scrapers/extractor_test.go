package scrapers

import (
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"

	"github.com/PuerkitoBio/goquery"
)

func domFrom(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("goquery parse error = %v", err)
	}
	return doc.Selection
}

func TestExtractFromDOMUsesArticleSelector(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".article"})
	dom := domFrom(t, `<html><body>
		<nav>menu</nav>
		<div class="article"><p>Real content</p></div>
	</body></html>`)

	got, err := ae.ExtractFromDOM(dom, "https://x", nil)
	if err != nil {
		t.Fatalf("ExtractFromDOM() error = %v", err)
	}
	if !strings.Contains(got, "Real content") {
		t.Fatalf("extracted %q, want it to contain the article content", got)
	}
	if strings.Contains(got, "menu") {
		t.Fatalf("extracted %q, want the nav excluded (article selector scopes output)", got)
	}
}

func TestExtractFromDOMFeedSelectorOverridesConfig(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".config-sel"})
	dom := domFrom(t, `<html><body>
		<div class="config-sel"><p>config content</p></div>
		<div class="feed-sel"><p>feed content</p></div>
	</body></html>`)

	got, err := ae.ExtractFromDOM(dom, "https://x", &models.Selectors{Article: ".feed-sel"})
	if err != nil {
		t.Fatalf("ExtractFromDOM() error = %v", err)
	}
	if !strings.Contains(got, "feed content") || strings.Contains(got, "config content") {
		t.Fatalf("extracted %q, want feed selector to win over config", got)
	}
}

func TestExtractFromDOMBlacklistRemovesElements(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".article", Blacklist: ".ad"})
	dom := domFrom(t, `<html><body>
		<div class="article"><p>keep this</p><div class="ad">buy now</div></div>
	</body></html>`)

	got, err := ae.ExtractFromDOM(dom, "https://x", nil)
	if err != nil {
		t.Fatalf("ExtractFromDOM() error = %v", err)
	}
	if strings.Contains(got, "buy now") {
		t.Fatalf("extracted %q, want blacklisted .ad removed", got)
	}
	if !strings.Contains(got, "keep this") {
		t.Fatalf("extracted %q, want non-blacklisted content kept", got)
	}
}

func TestExtractFromDOMCutoffTrimsTrailingSiblings(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".article", Cutoff: ".cutoff"})
	dom := domFrom(t, `<html><body>
		<div class="article"><p>before</p><hr class="cutoff"/><p>after</p></div>
	</body></html>`)

	got, err := ae.ExtractFromDOM(dom, "https://x", nil)
	if err != nil {
		t.Fatalf("ExtractFromDOM() error = %v", err)
	}
	if !strings.Contains(got, "before") {
		t.Fatalf("extracted %q, want content before cutoff kept", got)
	}
	if strings.Contains(got, "after") {
		t.Fatalf("extracted %q, want content after cutoff removed", got)
	}
}

func TestExtractFromDOMFallsBackToMarkdownWhenNoMatch(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".nonexistent"})
	dom := domFrom(t, `<html><body><p>Fallback body text</p></body></html>`)

	got, err := ae.ExtractFromDOM(dom, "https://x", nil)
	if err != nil {
		t.Fatalf("ExtractFromDOM() error = %v", err)
	}
	if !strings.Contains(got, "Fallback body text") {
		t.Fatalf("extracted %q, want markdown fallback containing body text", got)
	}
}

func TestGetSetSelectors(t *testing.T) {
	ae := NewArticleExtractor(&models.Selectors{Article: ".a"})
	if ae.GetSelectors().Article != ".a" {
		t.Fatalf("GetSelectors().Article = %q, want .a", ae.GetSelectors().Article)
	}
	ae.SetSelectors(&models.Selectors{Article: ".b"})
	if ae.GetSelectors().Article != ".b" {
		t.Fatalf("after SetSelectors, Article = %q, want .b", ae.GetSelectors().Article)
	}
}
