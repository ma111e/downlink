package notification

// RenderOption tunes how a page is rendered. The zero set of options produces a
// fully self-contained page (CSS inlined in a <style> block) — the default that
// Discord HTML attachments and the dev preview server rely on. The gh-pages
// publisher passes WithExternalCSS to link an external stylesheet instead.
type RenderOption func(*renderConfig)

type renderConfig struct {
	externalCSS bool
}

// WithExternalCSS makes a render emit a <link rel="stylesheet"> to a sibling
// .css file instead of inlining the static stylesheet. Only the published
// GitHub Pages site uses this; the .css files are written alongside the pages by
// the publisher. The (small, theme-dynamic) palette stays inline either way.
func WithExternalCSS() RenderOption {
	return func(c *renderConfig) { c.externalCSS = true }
}

func applyRenderOptions(opts []RenderOption) renderConfig {
	var c renderConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// styleFields returns the inline stylesheet body and the external <link> tag for
// a page. strippedCSS is the comment-stripped static CSS; cssFile is the sibling
// stylesheet name (e.g. "digest.css"). In external mode the body is empty and the
// link points at "./<cssFile>"; otherwise the body is the CSS and the link empty.
func (c renderConfig) styleFields(strippedCSS, cssFile string) (body, link string) {
	if c.externalCSS {
		return "", `<link rel="stylesheet" href="./` + cssFile + `">`
	}
	return strippedCSS, ""
}
