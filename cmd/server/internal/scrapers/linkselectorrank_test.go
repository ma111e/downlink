package scrapers

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

const blogIndexFixture = `<html><body>
	<nav class="nav">
		<a href="/">Home</a><a href="/about">About</a><a href="/contact">Contact</a>
	</nav>
	<div class="posts">
		<article class="card"><a class="title" href="/blog/alpha">Alpha</a><time datetime="2024-01-01">Jan 1, 2024</time></article>
		<article class="card"><a class="title" href="/blog/bravo">Bravo</a><time datetime="2024-02-01">Feb 1, 2024</time></article>
		<article class="card"><a class="title" href="/blog/charlie">Charlie</a><time datetime="2024-03-01">Mar 1, 2024</time></article>
		<article class="card"><a class="title" href="/blog/delta">Delta</a><time datetime="2024-04-01">Apr 1, 2024</time></article>
	</div>
	<footer class="foot">
		<a href="https://twitter.com/x">Twitter</a><a href="https://github.com/x">GitHub</a><a href="/privacy">Privacy</a>
	</footer>
</body></html>`

func docSel(t *testing.T, html string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return doc.Selection
}

func TestSuggestLinkSelectors_RanksPostListAboveChrome(t *testing.T) {
	base, _ := url.Parse("https://blog.example.com/")
	cands := SuggestLinkSelectors(docSel(t, blogIndexFixture), base, 5)
	if len(cands) == 0 {
		t.Fatal("expected at least one candidate")
	}
	top := cands[0]

	// The winning selector must scope the four blog posts, not nav/footer.
	if top.Count != 4 {
		t.Fatalf("top.Count = %d, want 4 (selector %q)", top.Count, top.LinksSelector)
	}
	for _, h := range top.SampleHrefs {
		if !strings.Contains(h, "/blog/") {
			t.Errorf("sample href %q is not a blog post (selector %q)", h, top.LinksSelector)
		}
	}
	// Inferred from the repeating structure.
	if top.URLFilter != "/blog/" {
		t.Errorf("URLFilter = %q, want /blog/", top.URLFilter)
	}
	if top.DateSelector == "" {
		t.Errorf("expected a date_selector to be inferred from the block")
	}
}

func TestSuggestLinkSelectors_DateSelectorMatchesBlockDate(t *testing.T) {
	base, _ := url.Parse("https://blog.example.com/")
	cands := SuggestLinkSelectors(docSel(t, blogIndexFixture), base, 5)
	if len(cands) == 0 {
		t.Fatal("expected candidates")
	}
	// The proposed links_selector + date_selector must actually drive parseLinkList
	// to real per-post dates (end-to-end check of the two inferred selectors).
	top := cands[0]
	items, err := parseLinkList([]byte(blogIndexFixture), "https://blog.example.com/", top.LinksSelector, top.URLFilter, top.DateSelector)
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4", len(items))
	}
	for _, it := range items {
		if it.PublishedAt.Year() != 2024 {
			t.Errorf("item %q has no real date: %v", it.Link, it.PublishedAt)
		}
	}
}
