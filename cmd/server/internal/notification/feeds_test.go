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
						Technologies:    []string{"vpn", "firewall"},
						Products:        []string{"fortios"},
						Vendors:         []string{"fortinet"},
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
						Vendors:         []string{"omittedvendor"},
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
	// Duplicate non-canonical article is omitted (both its TLDR and its terms).
	if strings.Contains(body, "Should not appear") {
		t.Errorf("rss feed included a duplicate non-canonical article")
	}
	if strings.Contains(body, "omittedvendor") {
		t.Errorf("rss feed included terms from a duplicate non-canonical article")
	}
	// Per-article technology/product/vendor lists, machine-parsable.
	for _, want := range []string{
		`data-axis="technologies"`,
		`data-axis="products"`,
		`data-axis="vendors"`,
		`<data value="firewall">firewall</data>`,
		`<data value="fortios">fortios</data>`,
		`<data value="fortinet">fortinet</data>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rss feed missing per-article term markup %q", want)
		}
	}
	// Per-item window: RFC3339 datetimes + ISO-8601 duration (24h window sample).
	for _, want := range []string{
		`class="digest-window"`,
		`<time datetime="2026-04-24T12:00:00Z">`,
		`<time datetime="2026-04-25T12:00:00Z">`,
		`<data value="PT24H">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rss feed missing window markup %q", want)
		}
	}
	// Structured, namespaced per-item technology/product/vendor fields.
	for _, want := range []string{
		`xmlns:downlink="https://ma111e.github.io/downlink/ns"`,
		`<downlink:technologies>`,
		`<downlink:technology>vpn</downlink:technology>`,
		`<downlink:technology>firewall</downlink:technology>`,
		`<downlink:products>`,
		`<downlink:product>fortios</downlink:product>`,
		`<downlink:vendors>`,
		`<downlink:vendor>fortinet</downlink:vendor>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rss feed missing structured field markup %q", want)
		}
	}
	// The duplicate non-canonical article's vendor must not leak into the
	// structured fields either (already asserted absent from the body above).
	if strings.Contains(body, "<downlink:vendor>omittedvendor</downlink:vendor>") {
		t.Errorf("structured fields included a duplicate non-canonical article's vendor")
	}
}

func TestISO8601Duration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{24 * time.Hour, "PT24H"},
		{90 * time.Minute, "PT1H30M"},
		{45 * time.Minute, "PT45M"},
		{90 * time.Second, "PT1M30S"},
		{0, "PT0S"},
		{-5 * time.Minute, "PT0S"},
	}
	for _, c := range cases {
		if got := iso8601Duration(c.in); got != c.want {
			t.Errorf("iso8601Duration(%v) = %q, want %q", c.in, got, c.want)
		}
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

func TestPagesBaseURL(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		repoURL string
		want    string
	}{
		{"explicit base URL wins", "https://digests.example.com", "https://github.com/ma111e/downlink", "https://digests.example.com"},
		{"project site", "", "https://github.com/ma111e/downlink", "https://ma111e.github.io/downlink"},
		{"project site .git suffix", "", "https://github.com/ma111e/downlink.git", "https://ma111e.github.io/downlink"},
		{"user site", "", "https://github.com/ma111e/ma111e.github.io", "https://ma111e.github.io"},
		{"user site case-insensitive", "", "https://github.com/Ma111e/ma111e.github.io", "https://Ma111e.github.io"},
		{"unparseable repo URL yields empty", "", "not a url", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
				BaseURL: c.baseURL,
				RepoURL: c.repoURL,
			})
			if got := p.pagesBaseURL(); got != c.want {
				t.Errorf("pagesBaseURL() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestFeedURL(t *testing.T) {
	p := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL: "https://github.com/ma111e/downlink",
	})
	if want := "https://ma111e.github.io/downlink/rss.xml"; p.feedURL() != want {
		t.Errorf("feedURL() = %q, want %q", p.feedURL(), want)
	}

	empty := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{RepoURL: "bad"})
	if got := empty.feedURL(); got != "" {
		t.Errorf("feedURL() with unparseable repo = %q, want empty", got)
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

func TestArchiveIndexFeedLinks(t *testing.T) {
	feed := "https://ma111e.github.io/downlink/rss.xml"
	autodiscovery := `<link rel="alternate" type="application/rss+xml" title="Downlink Digests" href="` + feed + `">`
	footerLink := `<a href="` + feed + `">rss</a>`

	for _, layout := range []string{"default", "v2"} {
		t.Run(layout+"/with feed", func(t *testing.T) {
			html, err := RenderDigestIndex(layout, "", WithFeedURL(feed))
			if err != nil {
				t.Fatalf("RenderDigestIndex() error = %v", err)
			}
			body := string(html)
			if !strings.Contains(body, autodiscovery) {
				t.Errorf("%s index missing autodiscovery link", layout)
			}
			if !strings.Contains(body, footerLink) {
				t.Errorf("%s index missing footer rss link", layout)
			}
		})
		t.Run(layout+"/without feed", func(t *testing.T) {
			html, err := RenderDigestIndex(layout, "")
			if err != nil {
				t.Fatalf("RenderDigestIndex() error = %v", err)
			}
			body := string(html)
			if strings.Contains(body, "application/rss+xml") {
				t.Errorf("%s index leaked an autodiscovery link with no feed URL", layout)
			}
			if strings.Contains(body, ">rss</a>") {
				t.Errorf("%s index leaked a footer rss link with no feed URL", layout)
			}
		})
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
