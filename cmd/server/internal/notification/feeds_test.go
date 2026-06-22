package notification

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
)

func sampleFeedDigests() []models.Digest {
	older := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	return []models.Digest{
		{
			Id:         "digest-new",
			CreatedAt:  newer,
			TimeWindow: 24 * time.Hour,
			Title:      "Newer Digest",
			Articles: []models.Article{
				{Id: "art-1", Title: "Critical CVE disclosed", Link: "https://example.com/1", PublishedAt: newer},
				{Id: "art-2", Title: "Duplicate coverage", Link: "https://example.com/2", PublishedAt: newer},
			},
			DigestAnalyses: []models.DigestAnalysis{
				{
					ArticleId: "art-1",
					Analysis: &models.ArticleAnalysis{
						ArticleId:       "art-1",
						ImportanceScore: 95,
						Tldr:            "A severe vulnerability was disclosed today.",
						KeyPoints:       []string{"Affects all versions", "Patch available now"},
					},
				},
				{
					ArticleId:           "art-2",
					DuplicateGroup:      "grp-1",
					IsMostComprehensive: false,
					Analysis: &models.ArticleAnalysis{
						ArticleId:       "art-2",
						ImportanceScore: 40,
						Tldr:            "Duplicate story that should be omitted.",
						KeyPoints:       []string{"Should not appear"},
					},
				},
			},
		},
		{
			Id:         "digest-old",
			CreatedAt:  older,
			TimeWindow: 24 * time.Hour,
			Title:      "Older Digest",
			Articles: []models.Article{
				{Id: "art-3", Title: "Older headline", Link: "https://example.com/3", PublishedAt: older},
			},
			DigestAnalyses: []models.DigestAnalysis{
				{
					ArticleId: "art-3",
					Analysis: &models.ArticleAnalysis{
						ArticleId:       "art-3",
						ImportanceScore: 70,
						Tldr:            "An older but relevant development.",
						KeyPoints:       []string{"Background context"},
					},
				},
			},
		},
	}
}

func TestBuildDigestFeedsContent(t *testing.T) {
	rss, err := BuildDigestFeeds(sampleFeedDigests(), "digests", "https://user.github.io")
	if err != nil {
		t.Fatalf("BuildDigestFeeds() error = %v", err)
	}

	// The feed must be well-formed XML.
	if err := xml.Unmarshal(rss, new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("rss is not valid XML: %v", err)
	}

	body := string(rss)
	// Both digest titles present.
	for _, want := range []string{"Newer Digest", "Older Digest"} {
		if !strings.Contains(body, want) {
			t.Errorf("rss feed missing digest title %q", want)
		}
	}
	// Per-article TLDR + key points present.
	for _, want := range []string{
		"A severe vulnerability was disclosed today.",
		"Affects all versions",
		"Patch available now",
		"An older but relevant development.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rss feed missing article content %q", want)
		}
	}
	// Absolute link to the newer digest page.
	if want := "https://user.github.io/digests/downlink-digest-2026-04-24_1200.html"; !strings.Contains(body, want) {
		t.Errorf("rss feed missing absolute digest link %q", want)
	}
	// Duplicate non-canonical article is omitted.
	if strings.Contains(body, "Should not appear") {
		t.Errorf("rss feed included a duplicate non-canonical article")
	}
}

func TestBuildDigestFeedsEmpty(t *testing.T) {
	rss, err := BuildDigestFeeds(nil, "digests", "https://user.github.io")
	if err != nil {
		t.Fatalf("BuildDigestFeeds() error = %v", err)
	}
	if len(rss) == 0 {
		t.Fatal("expected non-empty feed for empty digest list")
	}
	if err := xml.Unmarshal(rss, new(struct{ XMLName xml.Name })); err != nil {
		t.Fatalf("empty rss is not valid XML: %v", err)
	}
}

func TestBuildDigestFeedsRelativeLinksWhenNoBaseURL(t *testing.T) {
	rss, err := BuildDigestFeeds(sampleFeedDigests(), "digests", "")
	if err != nil {
		t.Fatalf("BuildDigestFeeds() error = %v", err)
	}
	if want := "/digests/downlink-digest-2026-04-24_1200.html"; !strings.Contains(string(rss), want) {
		t.Errorf("rss feed missing relative digest link %q", want)
	}
}

func TestMergeDigestsNewestFirst(t *testing.T) {
	t1 := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 4, 23, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC)
	in := []models.Digest{
		{Id: "a", CreatedAt: t1},
		{Id: "c", CreatedAt: t3},
		{Id: "a", CreatedAt: t1}, // duplicate Id
		{Id: "b", CreatedAt: t2},
	}
	got := mergeDigestsNewestFirst(in, 2)
	if len(got) != 2 {
		t.Fatalf("expected cap of 2, got %d", len(got))
	}
	if got[0].Id != "c" || got[1].Id != "b" {
		t.Errorf("expected newest-first [c b], got [%s %s]", got[0].Id, got[1].Id)
	}
}

func TestJoinURL(t *testing.T) {
	cases := []struct {
		base     string
		segments []string
		want     string
	}{
		{"https://user.github.io", []string{"digests", "x.html"}, "https://user.github.io/digests/x.html"},
		{"https://user.github.io/", []string{"digests", ""}, "https://user.github.io/digests"},
		{"", []string{"digests", "x.html"}, "/digests/x.html"},
		{"", []string{"", ""}, "/"},
	}
	for _, c := range cases {
		if got := joinURL(c.base, c.segments...); got != c.want {
			t.Errorf("joinURL(%q, %v) = %q, want %q", c.base, c.segments, got, c.want)
		}
	}
}
