package manager

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/models"
)

func TestApplyProfiles(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewFeedManager(db)

	// A stored feed the profile will reference by URL. Store it directly with the
	// same domain id ApplyProfiles will resolve, avoiding scraper registration.
	feedID, err := generateFeedId("https://example.com/rss")
	if err != nil {
		t.Fatalf("generateFeedId: %v", err)
	}
	enabled := true
	if err := db.StoreFeed(models.Feed{Id: feedID, URL: "https://example.com/rss", Type: "rss", Enabled: &enabled}); err != nil {
		t.Fatalf("StoreFeed: %v", err)
	}

	persona := "Explain like the reader is a journalist."
	pf := &models.ProfilesFile{
		Profiles: []models.ProfileConfig{
			{
				Slug:  "press",
				Name:  "Press Desk",
				Feeds: []string{"https://example.com/rss"},
				Editorial: &models.ProfileEditorial{
					Persona:    persona,
					Categories: []models.CategoryDef{{Name: "scoop"}},
				},
			},
		},
	}

	res, err := m.ApplyProfiles(pf)
	if err != nil {
		t.Fatalf("ApplyProfiles: %v", err)
	}
	if len(res.Upserted) != 1 || res.Upserted[0] != "press" {
		t.Fatalf("unexpected apply result: %+v", res)
	}

	// Editorial round-trips through the DB JSON column.
	p, err := db.GetProfile("press")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p.Editorial == nil || p.Editorial.Persona != persona {
		t.Errorf("profile editorial not persisted: %+v", p.Editorial)
	}
	if p.Editorial == nil || len(p.Editorial.Categories) != 1 || p.Editorial.Categories[0].Name != "scoop" {
		t.Errorf("profile categories not persisted: %+v", p.Editorial)
	}

	// The profile's feed pool resolved the URL to the registered feed.
	feeds, err := db.ListProfileFeeds("press")
	if err != nil {
		t.Fatalf("ListProfileFeeds: %v", err)
	}
	if len(feeds) != 1 || feeds[0].URL != "https://example.com/rss" {
		t.Errorf("expected press profile to own the example.com feed, got %+v", feeds)
	}

	// Re-applying is idempotent (no duplicate membership, still one feed).
	if _, err := m.ApplyProfiles(pf); err != nil {
		t.Fatalf("ApplyProfiles (second): %v", err)
	}
	feeds, _ = db.ListProfileFeeds("press")
	if len(feeds) != 1 {
		t.Errorf("expected 1 feed after re-apply, got %d", len(feeds))
	}
}

func TestProfileTopicSelection(t *testing.T) {
	db, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewFeedManager(db)

	// Distinct registrable domains so generateFeedId yields distinct ids.
	store2 := func(url string, topics []string) {
		id, err := generateFeedId(url)
		if err != nil {
			t.Fatalf("generateFeedId(%s): %v", url, err)
		}
		enabled := true
		if err := db.StoreFeed(models.Feed{Id: id, URL: url, Type: "rss", Enabled: &enabled}); err != nil {
			t.Fatalf("StoreFeed(%s): %v", url, err)
		}
		if err := db.SetFeedTopics(id, topics); err != nil {
			t.Fatalf("SetFeedTopics(%s): %v", url, err)
		}
	}
	const (
		aptURL  = "https://apt-feed.com/rss"
		newsURL = "https://news-feed.com/rss"
		bareURL = "https://bare-feed.com/rss" // no topics
	)
	store2(aptURL, []string{"apt"})
	store2(newsURL, []string{"news"})
	store2(bareURL, nil)

	urls := func(profile string) []string {
		feeds, err := db.ListProfileFeeds(profile)
		if err != nil {
			t.Fatalf("ListProfileFeeds(%s): %v", profile, err)
		}
		var out []string
		for _, f := range feeds {
			out = append(out, f.URL)
		}
		sort.Strings(out)
		return out
	}
	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	_, err = m.ApplyProfiles(&models.ProfilesFile{Profiles: []models.ProfileConfig{
		{Slug: "ti", Topics: []string{"apt"}},                                    // topic match
		{Slug: "everything"},                                                     // unscoped => all enabled
		{Slug: "news-clean", Topics: []string{"news"}, ExcludeFeeds: []string{newsURL}}, // topic minus explicit exclude
		{Slug: "pinned", Topics: []string{"apt"}, Feeds: []string{newsURL}},      // topic plus explicit include
	}})
	if err != nil {
		t.Fatalf("ApplyProfiles: %v", err)
	}

	if got := urls("ti"); !eq(got, []string{aptURL}) {
		t.Errorf("ti = %v, want [%s]", got, aptURL)
	}
	if got := urls("everything"); !eq(got, []string{aptURL, bareURL, newsURL}) {
		t.Errorf("everything = %v, want all enabled", got)
	}
	if got := urls("news-clean"); len(got) != 0 {
		t.Errorf("news-clean = %v, want empty (news feed excluded)", got)
	}
	if got := urls("pinned"); !eq(got, []string{aptURL, newsURL}) {
		t.Errorf("pinned = %v, want apt + explicitly-included news", got)
	}

	// Add a new feed tagged 'apt' and recompute from feeds-apply: the 'ti' profile
	// picks it up without re-applying the profile.
	const apt2URL = "https://apt-two.com/rss"
	store2(apt2URL, []string{"apt"})
	if err := m.RecomputeProfileFeeds(); err != nil {
		t.Fatalf("RecomputeProfileFeeds: %v", err)
	}
	if got := urls("ti"); !eq(got, []string{aptURL, apt2URL}) {
		t.Errorf("ti after new apt feed = %v, want both apt feeds", got)
	}
}
