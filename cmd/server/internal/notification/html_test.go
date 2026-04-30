package notification

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDigestIndexUsesManifest(t *testing.T) {
	htmlBytes, err := RenderDigestIndex()
	if err != nil {
		t.Fatalf("RenderDigestIndex() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		`data-manifest-url="manifest.json"`,
		`data-digest-base-url=""`,
		"var manifestURL = els.archive.getAttribute('data-manifest-url') || 'manifest.json';",
		"function digestURL(filename)",
		"fetch(manifestURL",
		"state.manifest.digests",
		"latest.started_at",
		"latest.summary",
		"latest.headlines",
		"d.must_count",
		"localStorage.getItem(PIN_KEY)",
		"data-layout=\"log\"",
		"data-layout=\"grid\"",
		"data-layout=\"timeline\"",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestIndex() missing %q:\n%s", want, html)
		}
	}

	for _, old := range []string{"displayDate", "providerType", "modelName", "articleSetHash"} {
		if strings.Contains(html, old) {
			t.Fatalf("RenderDigestIndex() still contains old manifest field %q", old)
		}
	}
}

func TestRenderDigestHTMLDoesNotIncludeManifestSwitcher(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, old := range []string{
		`id="digest-switcher"`,
		`data-digest-id=`,
		`data-article-set-hash=`,
		"fetch('manifest.json'",
		"articleSetHash",
	} {
		if strings.Contains(html, old) {
			t.Fatalf("RenderDigestHTML() still contains switcher fragment %q", old)
		}
	}
}

func TestRenderSwipeHTMLInjectsDigestAndArticles(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderSwipeHTML(digest, "downlink-digest-2026-04-24_1200.html")
	if err != nil {
		t.Fatalf("RenderSwipeHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		`window.__DL_DIGEST   = "downlink-digest-2026-04-24_1200.html";`,
		`window.__DL_ARTICLES = [{"n":1`,
		`"title":"Article B"`,
		`"priority":"MUST READ"`,
		`"tldr":"Article B tldr."`,
		`"briefOverview":"\u003cp\u003eArticle B brief overview.\u003c/p\u003e\n"`,
		`"standardSynthesis":"\u003cp\u003eArticle B standard synthesis.\u003c/p\u003e\n"`,
		`"comprehensiveSynthesis":"\u003cp\u003eArticle B comprehensive synthesis.\u003c/p\u003e\n"`,
		`"keyPoints":["Article B key point"]`,
		`"insights":["Article B insight"]`,
		`"referencedReports":[{"title":"Article B report","url":"https://example.com/report","publisher":"Example Labs","context":"Supporting source."}]`,
		`"title":"Article A"`,
		`"priority":"SHOULD READ"`,
		`function AnalysisTabs({ article })`,
		`Key Points`,
		`Reports`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderSwipeHTML() missing %q:\n%s", want, html)
		}
	}
}
