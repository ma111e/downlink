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
		"var manifestURL = list.getAttribute('data-manifest-url') || 'manifest.json';",
		"fetch(manifestURL",
		"m.digests",
		"a.href = e.filename",
		"e.displayDate",
		"e.providerType",
		"e.modelName",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestIndex() missing %q:\n%s", want, html)
		}
	}
}

func TestRenderDigestHTMLUsesManifestForSwitcher(t *testing.T) {
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML() error = %v", err)
	}
	html := string(htmlBytes)

	for _, want := range []string{
		`id="digest-switcher"`,
		`data-digest-id="digest-one"`,
		`data-article-set-hash="` + ArticleSetHash(digest) + `"`,
		"fetch('manifest.json'",
		"e.articleSetHash === hash",
		"opt.value = e.filename",
		"sel.hidden = false",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("RenderDigestHTML() missing %q", want)
		}
	}
}
