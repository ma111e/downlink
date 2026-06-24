package scrapers

import (
	"testing"
	"time"
)

const linkListFixture = `<html><body>
	<nav class="site-nav"><a href="/">Home</a><a href="/about">About</a></nav>
	<ul class="posts">
		<li><a href="/posts/first-post">First Post</a></li>
		<li><a href="/posts/second-post">  Second Post  </a></li>
		<li><a href="https://blog.example.com/posts/third-post">Third Post</a></li>
		<li><a href="/tags/go">A tag, not a post</a></li>
	</ul>
	<footer><a href="/privacy">Privacy</a></footer>
</body></html>`

func TestParseLinkList_SelectorScopesAndResolves(t *testing.T) {
	items, err := parseLinkList([]byte(linkListFixture), "https://blog.example.com/posts", "ul.posts li a", "", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4 (selector should exclude nav/footer)", len(items))
	}

	// Relative hrefs resolve against the page URL; titles come from anchor text (trimmed).
	if items[0].Link != "https://blog.example.com/posts/first-post" {
		t.Errorf("item[0].Link = %q", items[0].Link)
	}
	if items[0].Title != "First Post" {
		t.Errorf("item[0].Title = %q", items[0].Title)
	}
	if items[1].Title != "Second Post" {
		t.Errorf("item[1].Title = %q, want trimmed", items[1].Title)
	}
	// Already-absolute hrefs pass through unchanged.
	if items[2].Link != "https://blog.example.com/posts/third-post" {
		t.Errorf("item[2].Link = %q", items[2].Link)
	}
	// Every item carries a stable, non-empty id and no content (forces downstream scrape).
	for i, it := range items {
		if it.Id == "" {
			t.Errorf("item[%d] has empty Id", i)
		}
		if it.Content != "" {
			t.Errorf("item[%d] should have empty content, got %q", i, it.Content)
		}
	}
}

func TestParseLinkList_URLFilter(t *testing.T) {
	items, err := parseLinkList([]byte(linkListFixture), "https://blog.example.com/posts", "ul.posts li a", "/posts/", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (url_filter should drop the /tags/ link)", len(items))
	}
	for _, it := range items {
		if it.Title == "A tag, not a post" {
			t.Errorf("url_filter failed to drop %q", it.Link)
		}
	}
}

func TestParseLinkList_StableIDsAndDedup(t *testing.T) {
	first, err := parseLinkList([]byte(linkListFixture), "https://blog.example.com/posts", "ul.posts li a", "", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	second, err := parseLinkList([]byte(linkListFixture), "https://blog.example.com/posts", "ul.posts li a", "", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("non-deterministic item count: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Id != second[i].Id {
			t.Errorf("item[%d] id not stable across calls: %q vs %q", i, first[i].Id, second[i].Id)
		}
	}
}

func TestParseLinkList_DuplicateHrefsCollapse(t *testing.T) {
	html := `<ul class="posts">
		<li><a href="/posts/dup">One</a></li>
		<li><a href="/posts/dup">One again</a></li>
	</ul>`
	items, err := parseLinkList([]byte(html), "https://blog.example.com/posts", "ul.posts li a", "", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (duplicate hrefs collapse)", len(items))
	}
}

func TestParseLinkList_DateSelectorPerBlock(t *testing.T) {
	html := `<div class="list">
		<article class="post">
			<a href="/posts/a">Post A</a>
			<time datetime="2024-01-10">Jan 10, 2024</time>
		</article>
		<article class="post">
			<a href="/posts/b">Post B</a>
			<time datetime="2024-02-20">Feb 20, 2024</time>
		</article>
	</div>`
	items, err := parseLinkList([]byte(html), "https://blog.example.com", "article.post a", "", "time")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	// Each post gets the date from its OWN block, not the first one on the page.
	if y, m, d := items[0].PublishedAt.Date(); y != 2024 || m != 1 || d != 10 {
		t.Errorf("item[0] date = %v, want 2024-01-10", items[0].PublishedAt)
	}
	if y, m, d := items[1].PublishedAt.Date(); y != 2024 || m != 2 || d != 20 {
		t.Errorf("item[1] date = %v, want 2024-02-20", items[1].PublishedAt)
	}
}

func TestParseLinkList_DateSelectorMissingFallsBackToNow(t *testing.T) {
	html := `<div class="list">
		<article class="post"><a href="/posts/a">Post A</a></article>
	</div>`
	before := time.Now().Add(-time.Second)
	items, err := parseLinkList([]byte(html), "https://blog.example.com", "article.post a", "", "time")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].PublishedAt.Before(before) {
		t.Errorf("missing date should fall back to ~now, got %v", items[0].PublishedAt)
	}
}

func TestParseLinkList_TitleFallsBackToHref(t *testing.T) {
	html := `<ul class="posts"><li><a href="/posts/no-text"><img src="x.png"></a></li></ul>`
	items, err := parseLinkList([]byte(html), "https://blog.example.com/posts", "ul.posts li a", "", "")
	if err != nil {
		t.Fatalf("parseLinkList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Title != items[0].Link {
		t.Errorf("empty anchor text should fall back to href; got title %q link %q", items[0].Title, items[0].Link)
	}
}
