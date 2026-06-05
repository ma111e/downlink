package scrapers

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestSuggestSelectors_ArticleOutranksNav(t *testing.T) {
	prose := strings.Repeat("This is a real paragraph of article body text. ", 40)
	html := `<html><body>
		<nav class="site-nav"><a href="/a">Home</a><a href="/b">About</a><a href="/c">Posts</a><a href="/d">Tags</a><a href="/e">Contact</a></nav>
		<article class="post-content"><p>` + prose + `</p></article>
		<footer id="footer"><a href="/x">Privacy</a><a href="/y">Terms</a></footer>
	</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	cands := SuggestSelectors(doc.Selection, 8)
	if len(cands) == 0 {
		t.Fatal("no candidates returned")
	}

	top := cands[0]
	if top.Selector != "article.post-content" {
		t.Errorf("top candidate = %q, want article.post-content (got %+v)", top.Selector, cands)
	}
	if top.LinkDensity > 0.2 {
		t.Errorf("article link density = %.2f, expected low", top.LinkDensity)
	}

	// The nav must be ranked below the article (its score is dragged down by links).
	for i, c := range cands {
		if c.Selector == "nav.site-nav" && i == 0 {
			t.Errorf("nav ranked first; link density should have demoted it")
		}
	}
}

func TestSuggestSelectors_SkipsInvalidClassTokens(t *testing.T) {
	prose := strings.Repeat("Body text content here. ", 40)
	// Tailwind-style class with a colon must not produce an invalid selector.
	html := `<html><body><div class="md:flex prose"><p>` + prose + `</p></div></body></html>`
	doc, _ := goquery.NewDocumentFromReader(strings.NewReader(html))

	cands := SuggestSelectors(doc.Selection, 8)
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	for _, c := range cands {
		if strings.Contains(c.Selector, ":") {
			t.Errorf("selector %q contains an unescaped colon", c.Selector)
		}
	}
	// Should have used the valid "prose" class.
	if cands[0].Selector != "div.prose" {
		t.Errorf("top = %q, want div.prose", cands[0].Selector)
	}
}
