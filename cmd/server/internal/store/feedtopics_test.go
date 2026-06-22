package store

import (
	"sort"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestFeedTopics(t *testing.T) {
	s := newTestStore(t)

	en, dis := true, false
	if err := s.StoreFeed(models.Feed{Id: "a", URL: "https://a.test/rss", Enabled: &en}); err != nil {
		t.Fatalf("store a: %v", err)
	}
	if err := s.StoreFeed(models.Feed{Id: "b", URL: "https://b.test/rss", Enabled: &en}); err != nil {
		t.Fatalf("store b: %v", err)
	}
	if err := s.StoreFeed(models.Feed{Id: "c", URL: "https://c.test/rss", Enabled: &dis}); err != nil {
		t.Fatalf("store c (disabled): %v", err)
	}

	// SetFeedTopics lowercases, trims, and de-duplicates.
	if err := s.SetFeedTopics("a", []string{"APT", " apt ", "news"}); err != nil {
		t.Fatalf("SetFeedTopics a: %v", err)
	}
	got, err := s.ListFeedTopics("a")
	if err != nil {
		t.Fatalf("ListFeedTopics a: %v", err)
	}
	if want := []string{"apt", "news"}; !equalStrings(got, want) {
		t.Errorf("ListFeedTopics(a) = %v, want %v", got, want)
	}

	_ = s.SetFeedTopics("b", []string{"news"})
	_ = s.SetFeedTopics("c", []string{"apt"}) // disabled feed

	// FeedIDsByTopics is any-of and enabled-only (c is disabled, excluded).
	ids, err := s.FeedIDsByTopics([]string{"apt", "news"})
	if err != nil {
		t.Fatalf("FeedIDsByTopics: %v", err)
	}
	sort.Strings(ids)
	if want := []string{"a", "b"}; !equalStrings(ids, want) {
		t.Errorf("FeedIDsByTopics(apt,news) = %v, want %v (enabled only)", ids, want)
	}

	// Empty topic list returns nothing.
	if ids, _ := s.FeedIDsByTopics(nil); len(ids) != 0 {
		t.Errorf("FeedIDsByTopics(nil) = %v, want empty", ids)
	}

	// Re-setting replaces, does not append.
	_ = s.SetFeedTopics("a", []string{"vendor"})
	if got, _ := s.ListFeedTopics("a"); !equalStrings(got, []string{"vendor"}) {
		t.Errorf("after replace, ListFeedTopics(a) = %v, want [vendor]", got)
	}

	// Enabled-only helper.
	enabled, err := s.ListEnabledFeedIDs()
	if err != nil {
		t.Fatalf("ListEnabledFeedIDs: %v", err)
	}
	sort.Strings(enabled)
	if !equalStrings(enabled, []string{"a", "b"}) {
		t.Errorf("ListEnabledFeedIDs = %v, want [a b]", enabled)
	}

	// ListFeeds populates each feed's topics (the export path reads this).
	feeds, err := s.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds: %v", err)
	}
	byID := map[string][]string{}
	for _, f := range feeds {
		byID[f.Id] = f.Topics
	}
	if !equalStrings(byID["a"], []string{"vendor"}) { // a was re-set to [vendor] above
		t.Errorf("ListFeeds topics for a = %v, want [vendor]", byID["a"])
	}
	if !equalStrings(byID["b"], []string{"news"}) {
		t.Errorf("ListFeeds topics for b = %v, want [news]", byID["b"])
	}
}

func TestListAllTopics(t *testing.T) {
	s := newTestStore(t)
	en := true
	for _, id := range []string{"a", "b", "c"} {
		if err := s.StoreFeed(models.Feed{Id: id, URL: "https://" + id + ".test/rss", Enabled: &en}); err != nil {
			t.Fatalf("store %s: %v", id, err)
		}
	}

	// Empty when no topics are set.
	if got, err := s.ListAllTopics(); err != nil || len(got) != 0 {
		t.Fatalf("ListAllTopics (empty) = %v, %v, want []", got, err)
	}

	_ = s.SetFeedTopics("a", []string{"malware", "news"})
	_ = s.SetFeedTopics("b", []string{"news", "privacy"}) // "news" overlaps with a

	// Distinct, sorted across all feeds.
	got, err := s.ListAllTopics()
	if err != nil {
		t.Fatalf("ListAllTopics: %v", err)
	}
	if want := []string{"malware", "news", "privacy"}; !equalStrings(got, want) {
		t.Errorf("ListAllTopics = %v, want %v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
