package mappers

import (
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
	"gorm.io/datatypes"
)

func TestFeedToProtoNilIsNil(t *testing.T) {
	got, err := FeedToProto(nil)
	if err != nil {
		t.Fatalf("FeedToProto(nil) error = %v", err)
	}
	if got != nil {
		t.Fatalf("FeedToProto(nil) = %v, want nil", got)
	}
}

func TestFeedRoundTripPreservesFields(t *testing.T) {
	enabled := true
	gid := "grp-1"
	when := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	in := &models.Feed{
		Id:        "f1",
		URL:       "https://example.com/rss",
		Type:      "rss",
		Title:     "Example",
		LastFetch: when,
		Topics:    []string{"sec", "ai"},
		Enabled:   &enabled,
		GroupId:   &gid,
		Scraper:   datatypes.JSONMap{"mode": "http", "wait": "load"},
	}

	pf, err := FeedToProto(in)
	if err != nil {
		t.Fatalf("FeedToProto() error = %v", err)
	}
	out, err := FeedToModel(pf)
	if err != nil {
		t.Fatalf("FeedToModel() error = %v", err)
	}

	if out.Id != "f1" || out.URL != in.URL || out.Type != "rss" || out.Title != "Example" {
		t.Errorf("scalar fields lost: %+v", out)
	}
	if !out.LastFetch.Equal(when) {
		t.Errorf("LastFetch = %v, want %v", out.LastFetch, when)
	}
	if len(out.Topics) != 2 || out.Topics[0] != "sec" || out.Topics[1] != "ai" {
		t.Errorf("Topics = %v, want [sec ai]", out.Topics)
	}
	if out.Enabled == nil || !*out.Enabled {
		t.Errorf("Enabled = %v, want true pointer", out.Enabled)
	}
	if out.GroupId == nil || *out.GroupId != "grp-1" {
		t.Errorf("GroupId = %v, want grp-1", out.GroupId)
	}
	if out.Scraper["mode"] != "http" || out.Scraper["wait"] != "load" {
		t.Errorf("Scraper map lost: %v", out.Scraper)
	}
}

func TestFeedConfigRoundTripPreservesSelectorsTriggersOptions(t *testing.T) {
	in := &models.FeedConfig{
		URL:     "https://example.com/rss",
		Title:   "Ex",
		Enabled: true,
		Scraper: models.ScraperConfig{
			Type:      "html",
			Scraping:  "browser",
			Headers:   map[string]string{"User-Agent": "dlk"},
			Selectors: &models.Selectors{Article: ".post", Cutoff: ".ad", Blacklist: ".promo"},
			Options:   map[string]any{"timeout": "30s"},
		},
	}

	pf, err := FeedConfigToProto(in)
	if err != nil {
		t.Fatalf("FeedConfigToProto() error = %v", err)
	}
	out, err := FeedConfigToModel(pf)
	if err != nil {
		t.Fatalf("FeedConfigToModel() error = %v", err)
	}

	if out.URL != in.URL || out.Title != "Ex" || !out.Enabled {
		t.Errorf("scalar fields lost: %+v", out)
	}
	if out.Scraper.Type != "html" || out.Scraper.Scraping != "browser" {
		t.Errorf("scraper type/mode lost: %+v", out.Scraper)
	}
	if out.Scraper.Headers["User-Agent"] != "dlk" {
		t.Errorf("headers lost: %v", out.Scraper.Headers)
	}
	if out.Scraper.Selectors == nil || out.Scraper.Selectors.Article != ".post" ||
		out.Scraper.Selectors.Cutoff != ".ad" || out.Scraper.Selectors.Blacklist != ".promo" {
		t.Errorf("selectors lost: %+v", out.Scraper.Selectors)
	}
	if out.Scraper.Options["timeout"] != "30s" {
		t.Errorf("options lost: %v", out.Scraper.Options)
	}
}

func TestFeedDiagnosisInvalidUTF8Sentinel(t *testing.T) {
	// nil pointer must encode to -1 in proto, then decode back to nil.
	nilDiag := models.FeedDiagnosis{URL: "u"}
	if got := FeedDiagnosisToProto(nilDiag).InvalidUtf8At; got != -1 {
		t.Fatalf("proto InvalidUtf8At = %d, want -1 for nil pointer", got)
	}
	back := FeedDiagnosisToModel(FeedDiagnosisToProto(nilDiag))
	if back.InvalidUTF8At != nil {
		t.Fatalf("round-tripped InvalidUTF8At = %v, want nil", back.InvalidUTF8At)
	}

	// A concrete offset must survive the round-trip.
	at := 42
	setDiag := models.FeedDiagnosis{URL: "u", InvalidUTF8At: &at}
	if got := FeedDiagnosisToProto(setDiag).InvalidUtf8At; got != 42 {
		t.Fatalf("proto InvalidUtf8At = %d, want 42", got)
	}
	back = FeedDiagnosisToModel(FeedDiagnosisToProto(setDiag))
	if back.InvalidUTF8At == nil || *back.InvalidUTF8At != 42 {
		t.Fatalf("round-tripped InvalidUTF8At = %v, want 42", back.InvalidUTF8At)
	}
}

func TestFeedItemRoundTrip(t *testing.T) {
	when := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	in := &models.FeedItem{
		Id: "i1", Title: "T", Content: "C", Link: "https://x/1",
		PublishedAt: when, Tags: []string{"a"}, Category: "sec", HeroImage: "img.png",
	}
	out := FeedItemToModel(FeedItemToProto(in))
	if out.Id != "i1" || out.Title != "T" || out.Content != "C" || out.Link != in.Link ||
		!out.PublishedAt.Equal(when) || out.Category != "sec" || out.HeroImage != "img.png" {
		t.Fatalf("FeedItem round-trip lost fields: %+v", out)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "a" {
		t.Fatalf("tags = %v, want [a]", out.Tags)
	}
}

func TestFeedItemNilIsNil(t *testing.T) {
	if FeedItemToProto(nil) != nil {
		t.Error("FeedItemToProto(nil) != nil")
	}
	if FeedItemToModel(nil) != nil {
		t.Error("FeedItemToModel(nil) != nil")
	}
}

func TestFeedResultCarriesError(t *testing.T) {
	in := &models.FeedResult{Feed: models.Feed{Id: "f1"}, Error: errTest}
	pf, err := FeedResultToProto(in)
	if err != nil {
		t.Fatalf("FeedResultToProto() error = %v", err)
	}
	if pf.Error != "boom" {
		t.Fatalf("proto error = %q, want boom", pf.Error)
	}
	out, err := FeedResultToModel(pf)
	if err != nil {
		t.Fatalf("FeedResultToModel() error = %v", err)
	}
	if out.Error == nil || out.Error.Error() != "boom" {
		t.Fatalf("model error = %v, want boom", out.Error)
	}
}

var errTest = &stringError{"boom"}

type stringError struct{ s string }

func (e *stringError) Error() string { return e.s }
