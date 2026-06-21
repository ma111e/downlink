package main

import (
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestDiffScraper_NoChange(t *testing.T) {
	a := models.ScraperConfig{Type: "rss", Selectors: &models.Selectors{Article: "article"}}
	b := models.ScraperConfig{Type: "rss", Selectors: &models.Selectors{Article: "article"}}
	if got := diffScraper(a, b); len(got) != 0 {
		t.Errorf("expected no changes, got %v", got)
	}
}

func TestDiffScraper_ScrapingMode(t *testing.T) {
	a := models.ScraperConfig{Type: "rss", Scraping: ""}
	b := models.ScraperConfig{Type: "rss", Scraping: "dynamic"}
	got := diffScraper(a, b)
	if len(got) != 1 {
		t.Fatalf("expected 1 change, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "scraping") || !strings.Contains(got[0], "dynamic") {
		t.Errorf("unexpected diff line: %q", got[0])
	}
}

func TestDiffScraper_Selector(t *testing.T) {
	a := models.ScraperConfig{Selectors: &models.Selectors{Article: "div.old"}}
	b := models.ScraperConfig{Selectors: &models.Selectors{Article: "article.new"}}
	got := diffScraper(a, b)
	if len(got) != 1 || !strings.Contains(got[0], "selectors.article") {
		t.Fatalf("expected article selector diff, got %v", got)
	}
	if !strings.Contains(got[0], "div.old") || !strings.Contains(got[0], "article.new") {
		t.Errorf("diff should show old and new: %q", got[0])
	}
}

func TestDiffScraper_NilToSelector(t *testing.T) {
	a := models.ScraperConfig{}
	b := models.ScraperConfig{Selectors: &models.Selectors{Article: "article"}}
	got := diffScraper(a, b)
	if len(got) != 1 || !strings.Contains(got[0], "selectors.article") {
		t.Fatalf("expected article diff from nil selectors, got %v", got)
	}
}

func TestDiffScraper_Headers(t *testing.T) {
	a := models.ScraperConfig{Headers: map[string]string{"Referer": "https://old"}}
	b := models.ScraperConfig{Headers: map[string]string{
		"Referer":    "https://new",
		"User-Agent": "Mozilla",
	}}
	got := diffScraper(a, b)
	// Referer changed + User-Agent added = 2 changes.
	if len(got) != 2 {
		t.Fatalf("expected 2 header changes, got %d: %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "headers.Referer") || !strings.Contains(joined, "headers.User-Agent") {
		t.Errorf("missing header diff lines: %v", got)
	}
}

func TestDiffScraper_RemovedHeader(t *testing.T) {
	a := models.ScraperConfig{Headers: map[string]string{"X-Api-Key": "abc"}}
	b := models.ScraperConfig{}
	got := diffScraper(a, b)
	if len(got) != 1 || !strings.Contains(got[0], "headers.X-Api-Key") {
		t.Fatalf("expected removed-header diff, got %v", got)
	}
	if !strings.Contains(got[0], "(none)") {
		t.Errorf("removed value should render as (none): %q", got[0])
	}
}
