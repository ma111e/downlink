package notification

import (
	"strings"
	"testing"
)

// twoLayoutPeers is the common case: the classic default layout alongside the
// redesigned v2 layout, each in its own output subdir.
var twoLayoutPeers = []LayoutPeer{
	{Layout: "default", Subdir: "digests"},
	{Layout: "v2", Subdir: "digests-v2"},
}

// TestInjectLayoutSwitchDefault verifies the classic (default) page gets the
// pre-paint redirect/API script AND the "try the new layout" call-to-action,
// with the CTA targeting the v2 layout.
func TestInjectLayoutSwitchDefault(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><head><title>x</title></head><body>hi</body></html>`)
	out := string(injectLayoutSwitch(html, "default", twoLayoutPeers))

	for _, want := range []string{
		`window.__dlLayout`,          // API + redirect script present
		`"digests"`,                  // current subdir baked in
		`"digests-v2"`,               // peer subdir baked in
		`id="dl-layout-cta"`,         // CTA banner present on the classic layout
		`window.__dlLayout.go("v2")`, // CTA targets the redesigned layout
	} {
		if !strings.Contains(out, want) {
			t.Errorf("default-layout output missing %q\n---\n%s", want, out)
		}
	}
	// The redirect script must land in <head> so it runs before first paint.
	if hi := strings.Index(out, "<head>"); hi == -1 || !strings.Contains(out[hi:strings.Index(out, "</head>")], "window.__dlLayout") {
		t.Errorf("layout-switch script not injected into <head>")
	}
}

// TestInjectLayoutSwitchV2 verifies the v2 page gets the API/redirect script but
// NOT the call-to-action (that only nudges classic readers toward v2).
func TestInjectLayoutSwitchV2(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><head></head><body>hi</body></html>`)
	out := string(injectLayoutSwitch(html, "v2", twoLayoutPeers))

	if !strings.Contains(out, `window.__dlLayout`) {
		t.Errorf("v2 output missing the layout-switch API script")
	}
	if strings.Contains(out, `id="dl-layout-cta"`) {
		t.Errorf("v2 output should not carry the classic-layout call-to-action")
	}
}

// TestMaybeInjectLayoutSwitchSingle verifies a lone layout injects nothing:
// switching is inert unless at least two layouts coexist.
func TestMaybeInjectLayoutSwitchSingle(t *testing.T) {
	p := &GitHubPagesPublisher{}
	p.cfg.Layout = "default"
	p.SetLayoutPeers([]LayoutPeer{{Layout: "default", Subdir: "digests"}})

	html := []byte(`<html><head></head><body></body></html>`)
	if out := string(p.maybeInjectLayoutSwitch(html)); strings.Contains(out, "__dlLayout") {
		t.Errorf("single-layout publish should not inject the layout switch:\n%s", out)
	}
}
