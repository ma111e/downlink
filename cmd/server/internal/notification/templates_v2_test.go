package notification

import (
	"strings"
	"testing"
	"time"
)

// TestRenderV2Digest verifies the v2 layout renders the digest with its own template
// and, crucially, its own built assets: loadBuiltAsset("v2", "digest.js") must resolve
// the nested assets/v2/digest.js bundle rather than the default one.
func TestRenderV2Digest(t *testing.T) {
	digest := SampleDigest("v2-d1", time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC))

	out, err := RenderDigestHTML(digest, "v2", "dark")
	if err != nil {
		t.Fatalf("RenderDigestHTML(v2): %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`class="v2 v2-digest"`, // v2 template body
		"v2-detail-pane",       // v2-only structure
		"v2Select",             // v2 digest JS bundle (proves assets/v2/digest.js resolved)
		`data-theme="dark"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("v2 digest missing %q", want)
		}
	}
	// The default bundle must NOT leak in. handleClusterClick is a default-layout TOC
	// function with no v2 equivalent, so it's a stable "wrong bundle" sentinel. (Don't use
	// tour/glossary identifiers here — v2 now has its own tour + glossary panel too.)
	if strings.Contains(s, "handleClusterClick") {
		t.Error("v2 digest unexpectedly contains default digest bundle")
	}
}

// TestRenderV2Index verifies the v2 archive index renders its shell + v2 archive bundle.
func TestRenderV2Index(t *testing.T) {
	out, err := RenderDigestIndex("v2", "light")
	if err != nil {
		t.Fatalf("RenderDigestIndex(v2): %v", err)
	}
	s := string(out)
	for _, want := range []string{
		`class="v2 v2-archive"`,
		`id="archive"`,
		`data-theme="light"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("v2 index missing %q", want)
		}
	}
}

// TestRenderV2ExternalAssets verifies that in the published (external-asset) mode the
// v2 digest links the sibling digest.css/js by their canonical names — the layout-aware
// source lives under assets/v2/ but is written flat next to the HTML.
func TestRenderV2ExternalAssets(t *testing.T) {
	digest := SampleDigest("v2-d2", time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC))
	out, err := RenderDigestHTML(digest, "v2", "dark", WithExternalCSS())
	if err != nil {
		t.Fatalf("RenderDigestHTML(v2, external): %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `href="./digest.css"`) {
		t.Error("v2 external digest should link ./digest.css")
	}
	if !strings.Contains(s, `src="./digest.js"`) {
		t.Error("v2 external digest should link ./digest.js")
	}
}
