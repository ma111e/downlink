package notification

import (
	"strings"
	"testing"

	"github.com/ma111e/downlink/pkg/models"
)

func TestResolveTheme(t *testing.T) {
	if got := resolveTheme("light"); got != "light" {
		t.Errorf(`resolveTheme("light") = %q, want "light"`, got)
	}
	if got := resolveTheme(""); got != firstPaintTheme {
		t.Errorf(`resolveTheme("") = %q, want %q`, got, firstPaintTheme)
	}
	if got := resolveTheme("bogus"); got != firstPaintTheme {
		t.Errorf(`resolveTheme("bogus") = %q, want %q (fallback)`, got, firstPaintTheme)
	}
}

func TestRenderDigestHTMLAppliesProfileTheme(t *testing.T) {
	digest := models.Digest{Id: "d1", Title: "Test"}

	light, err := RenderDigestHTML(digest, "default", "light")
	if err != nil {
		t.Fatalf("RenderDigestHTML(light): %v", err)
	}
	if !strings.Contains(string(light), `data-theme="light"`) {
		t.Error(`profile theme "light" not baked into data-theme`)
	}

	def, err := RenderDigestHTML(digest, "default", "")
	if err != nil {
		t.Fatalf("RenderDigestHTML(default): %v", err)
	}
	if !strings.Contains(string(def), `data-theme="dark"`) {
		t.Error(`empty theme should fall back to data-theme="dark"`)
	}
}

func TestProfileSwitcherSnippet(t *testing.T) {
	s := profileSwitcherSnippet("beginner", "../")
	for _, want := range []string{
		`"../"`,           // rootPrefix baked in
		`"beginner"`,      // current slug baked in
		`profiles.json`,   // fetch target
		`dl-profile-switcher`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("switcher snippet missing %q", want)
		}
	}
}

func TestInjectProfileSwitcher(t *testing.T) {
	html := []byte("<html><body><h1>hi</h1></body></html>")
	out := string(injectProfileSwitcher(html, "red-team", "../"))
	if !strings.Contains(out, "dl-profile-switcher") {
		t.Fatal("switcher not injected")
	}
	// Must be inserted before the closing body tag, not after it.
	if strings.Index(out, "dl-profile-switcher") > strings.Index(out, "</body>") {
		t.Error("switcher injected after </body>, want before")
	}

	// No </body>: falls back to append.
	out2 := string(injectProfileSwitcher([]byte("<div>x</div>"), "a", "../"))
	if !strings.Contains(out2, "dl-profile-switcher") {
		t.Error("switcher not appended when </body> absent")
	}
}
