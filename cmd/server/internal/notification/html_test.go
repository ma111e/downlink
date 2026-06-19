package notification

import (
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"
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
	htmlBytes, err := RenderDigestIndex("dark")
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

func TestRenderSourcesPageListsEnabledFeeds(t *testing.T) {
	enabled := true
	disabled := false
	feeds := []models.Feed{
		{Title: "The Verge", URL: "https://www.theverge.com/rss/index.xml", Enabled: &enabled},
		{Title: "Defunct Blog", URL: "https://defunct.example.com/feed", Enabled: &disabled},
		{Title: "No Flag Feed", URL: "https://noflag.example.org/atom"}, // nil Enabled => treated as enabled
	}

	htmlBytes, err := RenderSourcesPage(feeds, "dark")
	if err != nil {
		t.Fatalf("RenderSourcesPage() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		"DOWNLINK // sources",
		"The Verge",
		"https://www.theverge.com/rss/index.xml",
		"theverge.com",
		"No Flag Feed",
		`href="index.html"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderSourcesPage() missing %q:\n%s", want, html)
		}
	}

	for _, omit := range []string{"Defunct Blog", "defunct.example.com"} {
		if strings.Contains(html, omit) {
			t.Fatalf("RenderSourcesPage() should omit disabled feed %q", omit)
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

	if !strings.Contains(html, `data-theme="dark"`) {
		t.Fatalf("RenderDigestHTML() did not bake the data-theme attribute")
	}

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

func TestRenderDigestHTMLBakesTheme(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))

	htmlBytes, err := RenderDigestHTML(digest, "mono")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	if !strings.Contains(string(htmlBytes), `data-theme="mono"`) {
		t.Fatalf("RenderDigestHTML() did not bake data-theme=\"mono\"")
	}

	// An unknown theme falls back to the dark default.
	htmlBytes, err = RenderDigestHTML(digest, "bogus")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	if !strings.Contains(string(htmlBytes), `data-theme="dark"`) {
		t.Fatalf("RenderDigestHTML() did not fall back to data-theme=\"dark\"")
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

func TestRenderDigestHTMLBakesFilterCountsAtBuildTime(t *testing.T) {
	// sampleDigest: article-b score 95 (Must Read), article-a score 80 (Should Read),
	// no categories. The counts must be baked into the spans, not filled by JS on load.
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		`MUST <span class="filter-count">1</span>`,
		`SHOULD <span class="filter-count">1</span>`,
		`MAY <span class="filter-count">0</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing baked priority count %q", want)
		}
	}

	// The load-time count IIFE and its dead helper classes must be gone.
	for _, gone := range []string{
		"update filter counts on load",
		"must-count",
		"should-count",
		"may-count",
		".toc-row-wrap[data-priority]",
	} {
		if strings.Contains(html, gone) {
			t.Fatalf("RenderDigestHTML() still contains removed count-on-load artifact %q", gone)
		}
	}
}

func TestRenderDigestHTMLPreCollapsesReportsAtBuildTime(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	category := "news"
	digest := models.Digest{
		Id:         "digest-reports",
		CreatedAt:  createdAt,
		TimeWindow: 24 * time.Hour,
		Articles: []models.Article{
			{Id: "article-x", Title: "Article X", Link: "https://example.com/x", PublishedAt: createdAt, CategoryName: &category},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				ArticleId: "article-x",
				Analysis: &models.ArticleAnalysis{
					ArticleId:       "article-x",
					ProviderType:    "openai",
					ModelName:       "gpt-test",
					ImportanceScore: 95,
					BriefOverview:   "Brief.",
					ReferencedReports: []models.ReferencedReport{
						{Title: "Primary source", URL: "https://example.com/p", Primary: true},
						{Title: "Secondary source", URL: "https://example.com/s", Primary: false},
					},
				},
			},
		},
	}

	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	// The non-primary report is pre-collapsed and a "+N more" button is pre-rendered server-side.
	for _, want := range []string{
		`<li class="report-item report-hidden">`,
		`class="report-show-more" data-more="+1 more" onclick="toggleReports(this)">+1 more`,
		`<span class="report-primary">PRIMARY</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing pre-collapsed report fragment %q:\n%s", want, html)
		}
	}

	// The category count is baked in too (one article in "news").
	if !strings.Contains(html, `news <span class="filter-count">1</span>`) {
		t.Fatalf("RenderDigestHTML() missing baked category count for news")
	}

	// The load-time report constructor must be gone (no DOM building on load).
	for _, gone := range []string{"createElement", "DOMContentLoaded"} {
		if strings.Contains(html, gone) {
			t.Fatalf("RenderDigestHTML() still contains removed load-time report constructor %q", gone)
		}
	}
}

func TestRenderDigestHTMLGlossaryMode(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	category := "news"
	digest := models.Digest{
		Id:         "digest-glossary",
		CreatedAt:  createdAt,
		TimeWindow: 24 * time.Hour,
		Articles: []models.Article{
			{Id: "article-b", Title: "Glossary Article", Link: "https://example.com/b", PublishedAt: createdAt, CategoryName: &category},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				ArticleId: "article-b",
				Analysis: &models.ArticleAnalysis{
					ArticleId:           "article-b",
					ProviderType:        "openai",
					ModelName:           "gpt-test",
					ImportanceScore:     95,
					BriefOverview:       "Brief.",
					GlossaryExplanation: "A flaw lets attackers run code on a server.",
					GlossaryTerms: []models.GlossaryTerm{
						{Term: "RCE", Definition: "Running your own commands on someone else's computer."},
					},
				},
			},
		},
	}

	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		// Nav switch + persistence plumbing appear when glossary content exists.
		`id="nav-glossary-toggle"`,
		`onclick="toggleGlossary()"`,
		`downlink.glossary`,
		`data-glossary`,
		// The per-article glossary block with explanation + terms.
		`class="panel-section glossary-block"`,
		`A flaw lets attackers run code on a server.`,
		`<dt class="glossary-term">RCE</dt>`,
		`Running your own commands on someone else&#39;s computer.`,
		// Hidden by default, revealed by the switch.
		`.glossary-block { display: none; }`,
		`html[data-glossary="on"] .glossary-block { display: block; }`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing glossary fragment %q:\n%s", want, html)
		}
	}
}

func TestRenderDigestHTMLGlossaryModal(t *testing.T) {
	createdAt := time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC)
	category := "news"
	digest := models.Digest{
		Id:         "digest-modal",
		CreatedAt:  createdAt,
		TimeWindow: 24 * time.Hour,
		Articles: []models.Article{
			{Id: "article-c", Title: "Modal Article", Link: "https://example.com/c", PublishedAt: createdAt, CategoryName: &category},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				ArticleId: "article-c",
				Analysis: &models.ArticleAnalysis{
					ArticleId:     "article-c",
					ProviderType:  "openai",
					ModelName:     "gpt-test",
					BriefOverview: "The crew deployed Cobalt Strike across the estate.",
				},
			},
		},
		DigestGlossary: []models.DigestGlossary{
			{
				DigestId: "digest-modal",
				EntryId:  "entry-1",
				Entry: &models.GlossaryEntry{
					Id:            "entry-1",
					NormalizedKey: models.NormalizeGlossaryKey("cobalt-strike"),
					Term:          "cobalt-strike",
					Kind:          models.GlossaryKindEntity,
					Definition:    "A commercial hacking toolkit often abused by attackers.",
					TagId:         "cobalt-strike",
				},
			},
		},
	}

	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		// The term→definition map is baked in, keyed by the normalized key.
		`var GLOSSARY = {`,
		`"cobalt strike":`,
		`A commercial hacking toolkit often abused by attackers.`,
		// The modal scaffold is present.
		`id="glossary-modal"`,
		`id="glossary-modal-def"`,
		// The entity name is highlighted in the prose (regex matches "Cobalt Strike" from slug).
		`<mark class="tag-hl">Cobalt Strike</mark>`,
		// Toggle is rendered since the digest has glossary content.
		`id="nav-glossary-toggle"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing glossary-modal fragment %q:\n%s", want, html)
		}
	}

	// The baked key must equal NormalizeGlossaryKey output for the term.
	if !strings.Contains(html, `"`+models.NormalizeGlossaryKey("Cobalt Strike")+`":`) {
		t.Fatal("baked glossary key does not match NormalizeGlossaryKey output")
	}
}

func TestRenderDigestHTMLNoGlossaryToggleWhenAbsent(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	if strings.Contains(html, `id="nav-glossary-toggle"`) {
		t.Fatal("RenderDigestHTML() rendered the glossary toggle for a digest with no glossary content")
	}
	if strings.Contains(html, `class="panel-section glossary-block"`) {
		t.Fatal("RenderDigestHTML() rendered a glossary block for a digest with no glossary content")
	}
}

func TestRenderSwipeHTMLInjectsDigestAndArticles(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderSwipeHTML(digest, "downlink-digest-2026-04-24_1200.html", "dark")
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
