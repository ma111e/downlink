package notification

import "strings"

// RenderOption tunes how a page is rendered. The zero set of options produces a
// fully self-contained page (CSS and JS inlined) — the default that Discord HTML
// attachments and the dev preview server rely on. The gh-pages publisher passes
// WithExternalCSS to link external assets instead.
type RenderOption func(*renderConfig)

type renderConfig struct {
	// externalAssets links sibling .css/.js files instead of inlining them. It
	// governs both stylesheets and scripts: the three render targets partition
	// cleanly (GitHub Pages links both; Discord and the dev server inline both).
	externalAssets bool
}

// WithExternalCSS makes a render link sibling .css/.js files (via <link> and
// <script src>) instead of inlining them. Only the published GitHub Pages site
// uses this; the files are written alongside the pages by the publisher. The
// small theme-dynamic palette and the blocking pre-paint scripts stay inline
// either way. The name is historical (it now covers scripts too).
func WithExternalCSS() RenderOption {
	return func(c *renderConfig) { c.externalAssets = true }
}

func applyRenderOptions(opts []RenderOption) renderConfig {
	var c renderConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// styleFields returns the inline stylesheet body and the external <link> tag for
// a page. strippedCSS is the (already minified) static CSS; cssFile is the
// sibling stylesheet name (e.g. "digest.css"). In external mode the body is empty
// and the link points at "./<cssFile>"; otherwise the body is the CSS and the
// link empty.
func (c renderConfig) styleFields(strippedCSS, cssFile string) (body, link string) {
	if c.externalAssets {
		return "", `<link rel="stylesheet" href="./` + cssFile + `">`
	}
	return strippedCSS, ""
}

// scriptFields returns the inline <script> body and the external <script src> tag
// for a page's bundle. js is the minified bundle; jsFile is the sibling filename
// (e.g. "sources.js"). In external mode the body is empty and a script tag points
// at "./<jsFile>"; otherwise the body is the inline-hardened JS and the tag empty.
func (c renderConfig) scriptFields(js, jsFile string) (body, srcTag string) {
	if c.externalAssets {
		return "", `<script type="module" src="./` + jsFile + `"></script>`
	}
	return hardenInlineScript(js), ""
}

// hardenInlineScript escapes any "</script" sequence in a bundle so it cannot
// close the surrounding inline <script> tag. Required for the self-contained
// Discord page, which embeds the bundle and is opened offline. The escaped form
// is equivalent JavaScript (a string like "</script>" is unaffected at runtime).
func hardenInlineScript(js string) string {
	return strings.ReplaceAll(js, "</script", `<\/script`)
}
