package notification

import (
	"strings"
	"testing"
	"time"
)

func TestHighlightTagsInSectionText(t *testing.T) {
	re := compileTagRegexp([]string{"cobalt-strike", "lazarus", "north-korea"})
	if re == nil {
		t.Fatal("compileTagRegexp returned nil for non-empty tags")
	}

	// Multi-word kebab tag matches spaced prose; single-word matches; case-insensitive.
	got := string(highlightPlain("Lazarus deployed Cobalt Strike against North Korea targets", re))
	for _, want := range []string{
		`<mark class="tag-hl">Lazarus</mark>`,
		`<mark class="tag-hl">Cobalt Strike</mark>`,
		`<mark class="tag-hl">North Korea</mark>`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("highlightPlain() missing %q in:\n%s", want, got)
		}
	}

	// HTML fragments: text is highlighted but tags/attributes (e.g. href) are untouched.
	frag := highlightHTMLFragment(`<p>See <a href="https://lazarus.example/cobalt-strike">Lazarus report</a></p>`, re)
	gotFrag := string(frag)
	if !strings.Contains(gotFrag, `<mark class="tag-hl">Lazarus</mark> report`) {
		t.Fatalf("highlightHTMLFragment() did not highlight link text:\n%s", gotFrag)
	}
	if !strings.Contains(gotFrag, `href="https://lazarus.example/cobalt-strike"`) {
		t.Fatalf("highlightHTMLFragment() corrupted the href attribute:\n%s", gotFrag)
	}

	// No tags → escape only, no marks, no panic.
	if got := string(highlightPlain("plain <b>text</b>", nil)); got != "plain &lt;b&gt;text&lt;/b&gt;" {
		t.Fatalf("highlightPlain(nil) = %q, want escaped passthrough", got)
	}
}

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

func TestRenderDigestHTMLShowsScoreTooltip(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	// article-b has rubric dimensions → the score bar is wrapped in a hover tooltip.
	for _, want := range []string{`class="score-tip"`, "Severity 4/4", "Specificity 4/4", "Credibility 3/4"} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing score tooltip fragment %q", want)
		}
	}

	// article-a has no dimensions → its score bar must not be wrapped (no empty tooltip).
	if strings.Contains(html, `data-tip=""`) {
		t.Fatal("RenderDigestHTML() rendered an empty score tooltip for an article without dimensions")
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
		`"referencedReports":[{"title":"Article B report","url":"https://example.com/report","publisher":"Example Labs","context":"Supporting source.","category":"","primary":false}]`,
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
