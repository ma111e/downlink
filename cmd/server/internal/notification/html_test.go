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
		"var manifestURL = els.archive.getAttribute('data-manifest-url') || 'manifest.json';",
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
