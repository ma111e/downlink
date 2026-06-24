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

// TestFeedConfig_Validate covers the hard requirements: every feed needs a title,
// and html feeds additionally need a date_selector.
func TestFeedConfig_Validate(t *testing.T) {
	htmlOpts := func(dateSelector any) map[string]any {
		opts := map[string]any{}
		if dateSelector != nil {
			opts["date_selector"] = dateSelector
		}
		return opts
	}

	cases := []struct {
		name    string
		cfg     FeedConfig
		wantErr bool
	}{
		{
			name: "rss with title",
			cfg:  FeedConfig{URL: "https://x.com/feed", Title: "X", Scraper: ScraperConfig{Type: "rss"}},
		},
		{
			name:    "empty title",
			cfg:     FeedConfig{URL: "https://x.com/feed", Title: "", Scraper: ScraperConfig{Type: "rss"}},
			wantErr: true,
		},
		{
			name:    "whitespace title",
			cfg:     FeedConfig{URL: "https://x.com/feed", Title: "   ", Scraper: ScraperConfig{Type: "rss"}},
			wantErr: true,
		},
		{
			name: "html with date_selector",
			cfg:  FeedConfig{URL: "https://x.com/blog", Title: "Blog", Scraper: ScraperConfig{Type: "html", Options: htmlOpts("time")}},
		},
		{
			name:    "html missing date_selector",
			cfg:     FeedConfig{URL: "https://x.com/blog", Title: "Blog", Scraper: ScraperConfig{Type: "html", Options: htmlOpts(nil)}},
			wantErr: true,
		},
		{
			name:    "html whitespace date_selector",
			cfg:     FeedConfig{URL: "https://x.com/blog", Title: "Blog", Scraper: ScraperConfig{Type: "html", Options: htmlOpts("  ")}},
			wantErr: true,
		},
		{
			name: "non-html without date_selector",
			cfg:  FeedConfig{URL: "https://x.com/feed", Title: "X", Scraper: ScraperConfig{Type: "atom"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr && err == nil {
				t.Errorf("Validate() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}
