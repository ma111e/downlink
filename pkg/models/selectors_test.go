package models

import (
	"testing"

	"gorm.io/datatypes"
)

func TestGetEffectiveSelectorsEmptyWhenNoSources(t *testing.T) {
	got := GetEffectiveSelectors(&Feed{}, nil)
	if got.Article != "" || got.Cutoff != "" || got.Blacklist != "" {
		t.Fatalf("got %+v, want all-empty selectors", got)
	}
}

func TestGetEffectiveSelectorsUsesConfigDefaults(t *testing.T) {
	cfg := &Selectors{Article: ".post", Cutoff: ".ad", Blacklist: ".promo"}
	got := GetEffectiveSelectors(&Feed{}, cfg)
	if got.Article != ".post" || got.Cutoff != ".ad" || got.Blacklist != ".promo" {
		t.Fatalf("got %+v, want config defaults", got)
	}
}

func TestGetEffectiveSelectorsFeedOverridesConfig(t *testing.T) {
	cfg := &Selectors{Article: ".post", Cutoff: ".ad", Blacklist: ".promo"}
	feed := &Feed{Scraper: datatypes.JSONMap{
		"selectors": Selectors{Article: ".feed-article"},
	}}
	got := GetEffectiveSelectors(feed, cfg)
	// Feed overrides only Article; Cutoff/Blacklist fall back to config.
	if got.Article != ".feed-article" {
		t.Errorf("Article = %q, want feed override .feed-article", got.Article)
	}
	if got.Cutoff != ".ad" || got.Blacklist != ".promo" {
		t.Errorf("Cutoff/Blacklist = %q/%q, want config fallback .ad/.promo", got.Cutoff, got.Blacklist)
	}
}

func TestGetEffectiveSelectorsIgnoresEmptyFeedFields(t *testing.T) {
	cfg := &Selectors{Article: ".post"}
	// Feed selectors present but Article empty -> must not blank out the config value.
	feed := &Feed{Scraper: datatypes.JSONMap{
		"selectors": Selectors{Cutoff: ".x"},
	}}
	got := GetEffectiveSelectors(feed, cfg)
	if got.Article != ".post" {
		t.Errorf("Article = %q, want config .post (empty feed field must not override)", got.Article)
	}
	if got.Cutoff != ".x" {
		t.Errorf("Cutoff = %q, want feed .x", got.Cutoff)
	}
}
