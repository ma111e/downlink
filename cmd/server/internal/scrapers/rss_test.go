package scrapers

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const rssBody = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel>
  <title>Test Feed</title>
  <item>
    <title>First Post</title>
    <link>https://example.com/1</link>
    <guid>guid-1</guid>
    <pubDate>Mon, 01 Jun 2026 12:00:00 GMT</pubDate>
    <description>Description one</description>
  </item>
  <item>
    <title>Second Post</title>
    <link>https://example.com/2</link>
    <pubDate>Tue, 02 Jun 2026 12:00:00 GMT</pubDate>
  </item>
</channel></rss>`

func TestRSSFetchParsesItems(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rssBody))
	}))
	defer srv.Close()

	s := NewRSSFeedScraper(nil)
	items, raw, err := s.Fetch(srv.URL, nil)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if raw == nil || raw.Status != http.StatusOK {
		t.Fatalf("raw response = %+v, want status 200", raw)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	// First item: GUID becomes the Id; description populates content.
	first := items[0]
	if first.Title != "First Post" || first.Link != "https://example.com/1" {
		t.Errorf("first item = %+v, want title/link parsed", first)
	}
	if first.Id != "guid-1" {
		t.Errorf("first Id = %q, want the GUID 'guid-1'", first.Id)
	}
	if first.Content != "Description one" {
		t.Errorf("first Content = %q, want the description", first.Content)
	}
	if first.PublishedAt.Year() != 2026 || first.PublishedAt.Month() != 6 {
		t.Errorf("first PublishedAt = %v, want 2026-06", first.PublishedAt)
	}

	// Second item: no GUID -> Id is the md5 hex of the link.
	wantID := fmt.Sprintf("%x", md5.Sum([]byte("https://example.com/2")))
	if items[1].Id != wantID {
		t.Errorf("second Id = %q, want md5(link) = %q", items[1].Id, wantID)
	}
}

func TestRSSFetchParseErrorReturnsRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not xml at all"))
	}))
	defer srv.Close()

	s := NewRSSFeedScraper(nil)
	items, raw, err := s.Fetch(srv.URL, nil)
	if err == nil {
		t.Fatal("Fetch() error = nil, want parse error")
	}
	if items != nil {
		t.Errorf("items = %v, want nil on parse failure", items)
	}
	// The raw body is preserved on parse failure for diagnostics.
	if raw == nil {
		t.Fatal("raw = nil on parse failure, want the raw response preserved")
	}
}
