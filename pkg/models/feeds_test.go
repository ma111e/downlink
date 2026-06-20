package models

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestFeedConfig_NestedScraperYAML verifies the consolidated scraper block parses:
// known fields land on typed fields, and unknown type-specific keys are captured
// by the inline Options map.
func TestFeedConfig_NestedScraperYAML(t *testing.T) {
	const doc = `
feeds:
  - url: https://blog.example.com/posts
    title: Linklist Blog
    enabled: true
    scraper:
      type: html
      scraping: static
      links_selector: "ul.posts li a"
      url_filter: "/posts/"
      selectors:
        article: div.post-content
        cutoff: .share
      headers:
        X-Api-Key: abc123
`
	var ff FeedsFile
	if err := yaml.Unmarshal([]byte(doc), &ff); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(ff.Feeds) != 1 {
		t.Fatalf("got %d feeds, want 1", len(ff.Feeds))
	}
	f := ff.Feeds[0]

	if f.URL != "https://blog.example.com/posts" || !f.Enabled {
		t.Errorf("top-level fields wrong: %+v", f)
	}
	if f.Scraper.Type != "html" {
		t.Errorf("Scraper.Type = %q, want html", f.Scraper.Type)
	}
	if f.Scraper.Scraping != "static" {
		t.Errorf("Scraper.Scraping = %q, want static", f.Scraper.Scraping)
	}
	if f.Scraper.Selectors == nil || f.Scraper.Selectors.Article != "div.post-content" {
		t.Errorf("Scraper.Selectors = %+v", f.Scraper.Selectors)
	}
	if f.Scraper.Headers["X-Api-Key"] != "abc123" {
		t.Errorf("Scraper.Headers = %+v", f.Scraper.Headers)
	}

	// Type-specific options are captured inline, not as typed fields.
	if got := f.Scraper.Options["links_selector"]; got != "ul.posts li a" {
		t.Errorf("Options[links_selector] = %v, want \"ul.posts li a\"", got)
	}
	if got := f.Scraper.Options["url_filter"]; got != "/posts/" {
		t.Errorf("Options[url_filter] = %v, want \"/posts/\"", got)
	}
	// Known keys must NOT leak into Options.
	for _, k := range []string{"type", "scraping", "selectors", "headers"} {
		if _, leaked := f.Scraper.Options[k]; leaked {
			t.Errorf("known key %q leaked into Options", k)
		}
	}
}
