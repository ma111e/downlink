package scrapers

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverFeedLinks_AllStrategies(t *testing.T) {
	body := []byte(`<!doctype html><html><head>
		<link rel="alternate" type="application/rss+xml" href="/feed.xml">
		<link rel="alternate" type="application/atom+xml" href="https://other.example/atom">
		<link rel="alternate" type="text/html" href="/printable">
	</head><body>
		<a href="/blog/rss">Subscribe via RSS</a>
		<a href="/about">About our breakfast menu</a>
		<a href="/news.atom">news</a>
	</body></html>`)

	got := discoverFeedLinks(body, "https://site.example/blog/")

	// <link> hits first (rank), resolved absolute.
	if got[0] != "https://site.example/feed.xml" {
		t.Errorf("expected feed.xml first, got %q (full: %v)", got[0], got)
	}
	if !contains(got, "https://other.example/atom") {
		t.Errorf("missing atom link autodiscovery: %v", got)
	}
	// text/html alternate must NOT be picked.
	if contains(got, "https://site.example/printable") {
		t.Errorf("text/html alternate should be ignored: %v", got)
	}
	// Anchor keyword + extension hits.
	if !contains(got, "https://site.example/blog/rss") {
		t.Errorf("missing anchor rss href: %v", got)
	}
	if !contains(got, "https://site.example/news.atom") {
		t.Errorf("missing .atom anchor: %v", got)
	}
	// False positive must not match.
	if contains(got, "https://site.example/about") {
		t.Errorf("breakfast/about should not match feed keywords: %v", got)
	}
	// Common paths appended.
	if !contains(got, "https://site.example/index.xml") {
		t.Errorf("missing a common-path candidate: %v", got)
	}
	// De-dupe: no duplicates.
	seen := map[string]bool{}
	for _, u := range got {
		if seen[u] {
			t.Errorf("duplicate candidate %q in %v", u, got)
		}
		seen[u] = true
	}
}

func TestLooksLikeFeed(t *testing.T) {
	cases := map[string]bool{
		"/feed":                true,
		"/feed.xml":            true,
		"https://x.com/rss":    true,
		"/news.atom":           true,
		"Subscribe via RSS":    true,
		"/about":               false,
		"breakfast":            false,
		"/feedback-form":       false, // "feed" not token-bounded
		"/posts/atomic-design": false, // "atom" not token-bounded
		"":                     false,
	}
	for in, want := range cases {
		if got := looksLikeFeed(in); got != want {
			t.Errorf("looksLikeFeed(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCommonPaths(t *testing.T) {
	got := commonPaths("https://site.example/section/page")
	if !contains(got, "https://site.example/feed") {
		t.Errorf("missing root /feed: %v", got)
	}
	if !contains(got, "https://site.example/rss.xml") {
		t.Errorf("missing root /rss.xml: %v", got)
	}
	// Path-relative variant.
	if !contains(got, "https://site.example/section/page/feed") {
		t.Errorf("missing path-relative /feed: %v", got)
	}
	// All absolute.
	for _, u := range got {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("candidate not absolute: %q", u)
		}
	}
}

func TestCommonPaths_InvalidURL(t *testing.T) {
	if got := commonPaths("not a url"); got != nil {
		t.Errorf("expected nil for hostless URL, got %v", got)
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "", "c", "b"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dedupe = %v, want %v", got, want)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
