package scrapers

import (
	"errors"
	"strings"
	"testing"
)

func TestAnalyzeFeedBody_ValidRSS(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel><title>X</title></channel></rss>`)
	raw := RawResponse{URL: "https://e.com/feed", Status: 200, ContentType: "application/rss+xml", Body: body}

	d := AnalyzeFeedBody(raw, nil, 3)

	if d.FeedTypeGuess != "rss" {
		t.Errorf("feed type guess = %q, want rss", d.FeedTypeGuess)
	}
	if d.InvalidUTF8At != nil {
		t.Errorf("InvalidUTF8At = %v, want nil for valid UTF-8", *d.InvalidUTF8At)
	}
	if d.DeclaredCharset != "utf-8" {
		t.Errorf("declared charset = %q, want utf-8", d.DeclaredCharset)
	}
	if !strings.Contains(d.Verdict, "valid rss feed, 3 items") {
		t.Errorf("verdict = %q, want valid-feed summary", d.Verdict)
	}
}

func TestAnalyzeFeedBody_AntiBotHTML(t *testing.T) {
	body := []byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>Checking your browser before accessing.</body></html>`)
	raw := RawResponse{URL: "https://e.com/feed", Status: 403, ContentType: "text/html", Body: body}

	d := AnalyzeFeedBody(raw, errors.New("Failed to detect feed type"), 0)

	if d.FeedTypeGuess != "html" {
		t.Errorf("feed type guess = %q, want html", d.FeedTypeGuess)
	}
	if !strings.Contains(strings.ToLower(d.Verdict), "cloudflare") {
		t.Errorf("verdict = %q, want a Cloudflare mention", d.Verdict)
	}
}

func TestAnalyzeFeedBody_InvalidUTF8(t *testing.T) {
	// "café" with é encoded as Latin-1 0xe9 inside an XML feed.
	body := []byte("<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><rss><title>caf\xe9</title></rss>")
	offsetOfBadByte := strings.IndexByte(string(body), 0xe9)
	raw := RawResponse{URL: "https://e.com/feed", Status: 200, ContentType: "text/xml", Body: body}

	d := AnalyzeFeedBody(raw, errors.New("XML syntax error: invalid UTF-8"), 0)

	if d.InvalidUTF8At == nil {
		t.Fatal("InvalidUTF8At = nil, want an offset")
	}
	if *d.InvalidUTF8At != offsetOfBadByte {
		t.Errorf("InvalidUTF8At = %d, want %d", *d.InvalidUTF8At, offsetOfBadByte)
	}
	if d.DeclaredCharset != "iso-8859-1" {
		t.Errorf("declared charset = %q, want iso-8859-1", d.DeclaredCharset)
	}
	if !strings.Contains(d.Verdict, "invalid UTF-8") || !strings.Contains(d.Verdict, "iso-8859-1") {
		t.Errorf("verdict = %q, want invalid-UTF-8 + charset", d.Verdict)
	}
	if d.HexDump == "" {
		t.Error("HexDump is empty, want bytes around the offset")
	}
}

func TestAnalyzeFeedBody_Empty(t *testing.T) {
	raw := RawResponse{URL: "https://e.com/feed", Status: 200, ContentType: "text/html", Body: []byte("   \n\t  ")}

	d := AnalyzeFeedBody(raw, errors.New("Failed to detect feed type"), 0)

	if d.FeedTypeGuess != "empty" {
		t.Errorf("feed type guess = %q, want empty", d.FeedTypeGuess)
	}
	if !strings.Contains(d.Verdict, "empty") {
		t.Errorf("verdict = %q, want empty-body mention", d.Verdict)
	}
}

func TestAnalyzeFeedBody_JSONFeed(t *testing.T) {
	body := []byte(`{"version":"https://jsonfeed.org/version/1","title":"X","items":[]}`)
	raw := RawResponse{URL: "https://e.com/feed.json", Status: 200, ContentType: "application/json", Body: body}

	d := AnalyzeFeedBody(raw, nil, 0)

	if d.FeedTypeGuess != "json-feed" {
		t.Errorf("feed type guess = %q, want json-feed", d.FeedTypeGuess)
	}
}

func TestGuessFeedType_Atom(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom"></feed>`)
	if got := guessFeedType(body); got != "atom" {
		t.Errorf("guessFeedType = %q, want atom", got)
	}
}

func TestFirstInvalidUTF8_Valid(t *testing.T) {
	if got := firstInvalidUTF8([]byte("héllo wörld")); got != -1 {
		t.Errorf("firstInvalidUTF8 = %d, want -1 for valid UTF-8", got)
	}
}
