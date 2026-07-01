package manager

import (
	"strings"
	"testing"
)

func TestFeedIDForURLReturnsDeterministicHex(t *testing.T) {
	id1, err := FeedIDForURL("https://feeds.example.com/rss")
	if err != nil {
		t.Fatalf("FeedIDForURL error = %v", err)
	}
	id2, err := FeedIDForURL("https://feeds.example.com/atom")
	if err != nil {
		t.Fatalf("FeedIDForURL error = %v", err)
	}
	// Same eTLD+1 → same id regardless of path.
	if id1 != id2 {
		t.Errorf("same domain, different paths: id1=%q id2=%q", id1, id2)
	}
	if len(id1) != 32 {
		t.Errorf("id len = %d, want 32 (MD5 hex)", len(id1))
	}
}

func TestFeedIDForURLDifferentDomainsProduceDifferentIds(t *testing.T) {
	a, err := FeedIDForURL("https://arstechnica.com/feed")
	if err != nil {
		t.Fatalf("FeedIDForURL(arstechnica) error = %v", err)
	}
	b, err := FeedIDForURL("https://wired.com/feed")
	if err != nil {
		t.Fatalf("FeedIDForURL(wired) error = %v", err)
	}
	if a == b {
		t.Errorf("different domains produced same id %q", a)
	}
}

func TestFeedIDForURLInvalidHostReturnsError(t *testing.T) {
	_, err := FeedIDForURL("https://localhost/feed")
	if err == nil {
		t.Error("FeedIDForURL(localhost) error = nil, want error (no eTLD+1)")
	}
}

func TestGenerateArticleIdUsesItemId(t *testing.T) {
	id := generateArticleId("feed1", "item-abc", "Some Title")
	if !strings.HasPrefix(id, "feed1:") {
		t.Errorf("id = %q, want prefix feed1:", id)
	}
	// Same itemId → same result regardless of title.
	id2 := generateArticleId("feed1", "item-abc", "Different Title")
	if id != id2 {
		t.Errorf("same itemId: id=%q id2=%q, want equal", id, id2)
	}
}

func TestGenerateArticleIdFallsBackToTitle(t *testing.T) {
	id := generateArticleId("feed1", "", "Article Title")
	if !strings.HasPrefix(id, "feed1:") {
		t.Errorf("id = %q, want prefix feed1:", id)
	}
	// Empty itemId with same title → same id.
	id2 := generateArticleId("feed1", "", "Article Title")
	if id != id2 {
		t.Errorf("same title: id=%q id2=%q, want equal", id, id2)
	}
	// Different title → different id.
	id3 := generateArticleId("feed1", "", "Other Title")
	if id == id3 {
		t.Errorf("different titles produced same id %q", id)
	}
}

func TestGenerateArticleIdFormatIsHashSuffix(t *testing.T) {
	id := generateArticleId("f1", "item-1", "")
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 || parts[0] != "f1" {
		t.Errorf("id format unexpected: %q", id)
	}
	if len(parts[1]) != 16 {
		t.Errorf("hash part len = %d, want 16 (8 bytes hex)", len(parts[1]))
	}
}
