package store

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func newTestStore(t *testing.T) *GormStore {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func boolPtr(b bool) *bool { return &b }

// TestSeedDefaultProfile confirms the migration seeds a default profile on a
// fresh DB and is idempotent across repeated runs.
func TestSeedDefaultProfile(t *testing.T) {
	s := newTestStore(t)

	p, err := s.GetProfile("default")
	if err != nil {
		t.Fatalf("default profile missing after seed: %v", err)
	}
	if p.Id != "default" || p.Name != "Default" {
		t.Errorf("unexpected default profile: %+v", p)
	}

	// Running the seed again must not create a duplicate or error.
	if err := s.seedDefaultProfile(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	profiles, err := s.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Errorf("expected 1 profile after idempotent seed, got %d", len(profiles))
	}
}

// TestSeedDefaultProfileBackfill simulates a pre-migration database (rows with
// no profile and no default profile) and verifies the seed attributes every
// enabled feed and every existing analysis/digest/run to "default".
func TestSeedDefaultProfileBackfill(t *testing.T) {
	s := newTestStore(t)

	// Insert an enabled feed and an article.
	if err := s.db.Create(&models.Feed{Id: "f1", URL: "https://x/rss", Enabled: boolPtr(true)}).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}
	if err := s.db.Create(&models.Article{Id: "a1", FeedId: "f1", Title: "t", PublishedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}

	// Simulate the pre-migration state: drop the seeded default profile + its
	// membership, and blank out profile_id on existing rows.
	if err := s.db.Exec("DELETE FROM profiles").Error; err != nil {
		t.Fatalf("clear profiles: %v", err)
	}
	if err := s.db.Exec("DELETE FROM profile_feeds").Error; err != nil {
		t.Fatalf("clear profile_feeds: %v", err)
	}
	if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{Id: "an1", ArticleId: "a1", ImportanceScore: 42}); err != nil {
		t.Fatalf("save analysis: %v", err)
	}
	if err := s.db.Create(&models.Digest{Id: "d1", CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create digest: %v", err)
	}
	if err := s.db.Exec("UPDATE article_analyses SET profile_id = ''").Error; err != nil {
		t.Fatalf("blank analysis profile: %v", err)
	}
	if err := s.db.Exec("UPDATE digests SET profile_id = ''").Error; err != nil {
		t.Fatalf("blank digest profile: %v", err)
	}

	if err := s.seedDefaultProfile(); err != nil {
		t.Fatalf("seedDefaultProfile: %v", err)
	}

	// The enabled feed must now belong to the default profile.
	feeds, err := s.ListProfileFeeds("default")
	if err != nil {
		t.Fatalf("ListProfileFeeds: %v", err)
	}
	if len(feeds) != 1 || feeds[0].Id != "f1" {
		t.Errorf("expected default profile to own f1, got %+v", feeds)
	}

	// No rows may be left without a profile.
	for _, table := range []string{"article_analyses", "digests"} {
		var n int64
		if err := s.db.Table(table).Where("profile_id = '' OR profile_id IS NULL").Count(&n).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s has %d rows with empty profile_id after backfill", table, n)
		}
	}

	// The backfilled analysis is retrievable under the default profile.
	got, err := s.GetArticleAnalysesBatch([]string{"a1"}, "default")
	if err != nil {
		t.Fatalf("GetArticleAnalysesBatch: %v", err)
	}
	if a := got["a1"]; a == nil || a.ImportanceScore != 42 {
		t.Errorf("expected default analysis for a1, got %+v", a)
	}
}

// TestProfileScopedAnalyses is the core of the shared-pool/per-lens model: the
// same article carries distinct analyses per profile, and the scoped getters
// return the right one.
func TestProfileScopedAnalyses(t *testing.T) {
	s := newTestStore(t)

	if err := s.db.Create(&models.Feed{Id: "f1", URL: "https://x/rss", Enabled: boolPtr(true)}).Error; err != nil {
		t.Fatalf("create feed: %v", err)
	}
	if err := s.db.Create(&models.Article{Id: "a1", FeedId: "f1", Title: "t", PublishedAt: time.Now()}).Error; err != nil {
		t.Fatalf("create article: %v", err)
	}
	if err := s.StoreProfile(models.Profile{Id: "alpha", Name: "Alpha"}); err != nil {
		t.Fatalf("store alpha: %v", err)
	}
	if err := s.StoreProfile(models.Profile{Id: "beta", Name: "Beta"}); err != nil {
		t.Fatalf("store beta: %v", err)
	}
	if err := s.SetProfileFeeds("alpha", []string{"f1"}); err != nil {
		t.Fatalf("set alpha feeds: %v", err)
	}
	if err := s.SetProfileFeeds("beta", []string{"f1"}); err != nil { // overlapping pool
		t.Fatalf("set beta feeds: %v", err)
	}

	// Two analyses for the same article, one per profile, different scores.
	if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{Id: "an-a", ArticleId: "a1", ProfileId: "alpha", ImportanceScore: 30}); err != nil {
		t.Fatalf("save alpha analysis: %v", err)
	}
	if err := s.SaveArticleAnalysis(&models.ArticleAnalysis{Id: "an-b", ArticleId: "a1", ProfileId: "beta", ImportanceScore: 90}); err != nil {
		t.Fatalf("save beta analysis: %v", err)
	}

	for profile, want := range map[string]int{"alpha": 30, "beta": 90} {
		got, err := s.GetArticleAnalysis("a1", profile)
		if err != nil {
			t.Fatalf("GetArticleAnalysis(%s): %v", profile, err)
		}
		if got.ImportanceScore != want {
			t.Errorf("profile %s: score = %d, want %d", profile, got.ImportanceScore, want)
		}
	}

	// ListArticles scoped to a profile must surface that profile's latest score.
	for profile, want := range map[string]int32{"alpha": 30, "beta": 90} {
		arts, err := s.ListArticles(models.ArticleFilter{ProfileId: profile, Unbounded: true})
		if err != nil {
			t.Fatalf("ListArticles(%s): %v", profile, err)
		}
		if len(arts) != 1 || arts[0].LatestImportanceScore == nil || *arts[0].LatestImportanceScore != want {
			t.Errorf("profile %s: ListArticles latest score = %v, want %d", profile, arts, want)
		}
	}
}
