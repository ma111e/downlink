package notification

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ma111e/downlink/pkg/digestlayouts"
	"github.com/ma111e/downlink/pkg/utils"
)

//go:embed templates/*/*.tmpl templates/landing.css templates/switcher.css
var notificationTemplateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// builtAssetsFS holds the Vite-built, pre-minified per-page assets (digest.css,
// digest.js, ...). The directory always exists thanks to the committed
// assets/PLACEHOLDER, so the embed compiles even before `make assets` has run;
// the real assets are produced by the web/ build and are gitignored.
//
//go:embed assets
var builtAssetsFS embed.FS

// staticAssets lists every file under static/ that should be written to the
// gh-pages output directory alongside digest HTML files.
var staticAssets = []string{
	"favicon.ico",
	"favicon-16.png",
	"favicon-32.png",
	"favicon-48.png",
	"icon-64.png",
	"icon-128.png",
	"icon-192.png",
	"icon-256.png",
	"icon-512.png",
	"icon-mono-256.png",
	"icon-white-256.png",
	"mark-ink.svg",
	"mark-warm.svg",
	"mark-white.svg",
	"site.webmanifest",
}

// stylesheetAssets lists the per-page CSS files the publisher writes alongside
// digest HTML when external CSS is enabled. Each name matches both a source
// .css under templates/<layout>/ and the "./<name>" link emitted by the
// corresponding page (see WithExternalCSS).
var stylesheetAssets = []string{
	"digest.css",
	"archive-index.css",
	"sources.css",
	"reports.css",
	"swipe.css",
}

// scriptAssets lists the per-page JS bundles the publisher writes alongside
// digest HTML when external assets are enabled. Each name matches both a
// Vite-built asset and the "./<name>" <script src> emitted by the corresponding
// page. Pages are migrated to external bundles incrementally; only the names
// listed here are written and linked.
var scriptAssets = []string{
	"sources.js",
	"archive-index.js",
	"reports.js",
	"digest.js",
	"swipe.js",
}

// writeStaticAsset writes a single embedded static file to dst if it doesn't
// already exist there (idempotent). Returns the relative path written so the
// caller can stage it in the git worktree.
func writeStaticAsset(name, dst string) error {
	data, err := staticFS.ReadFile("static/" + name)
	if err != nil {
		return fmt.Errorf("read embedded static/%s: %w", name, err)
	}
	existing, readErr := os.ReadFile(dst)
	if readErr == nil && string(existing) == string(data) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dst), err)
	}
	return os.WriteFile(dst, data, 0644)
}

// devTemplateDir, when non-empty, makes loadNotificationTemplate read templates
// from disk instead of the embedded FS. Used by the digest dev server so template
// edits show up on the next render with no recompile. Set via SetTemplateDir.
var devTemplateDir string

// SetTemplateDir switches template loading to read from dir on disk. Pass an empty
// string to fall back to the embedded templates. Intended for the dev preview server.
func SetTemplateDir(dir string) {
	devTemplateDir = dir
}

// assetDir, when non-empty, makes loadBuiltAsset read Vite-built assets from disk
// instead of the embedded builtAssetsFS. The dev server points it at the web/
// build output dir so `vite build --watch` rebuilds show up on the next render
// with no recompile. Set via SetAssetDir.
var assetDir string

// SetAssetDir switches built-asset loading to read from dir on disk. Pass an empty
// string to fall back to the embedded assets. Intended for the dev preview server.
func SetAssetDir(dir string) {
	assetDir = dir
}

// loadBuiltAsset reads a Vite-built asset (e.g. "digest.css", "digest.js") from
// the on-disk asset dir when set (dev), otherwise from the embedded FS. The
// content is already minified by the web build and is used verbatim.
func loadBuiltAsset(name string) (string, error) {
	if assetDir != "" {
		if b, err := os.ReadFile(filepath.Join(assetDir, name)); err == nil {
			return string(b), nil
		}
	}
	b, err := builtAssetsFS.ReadFile("assets/" + name)
	if err != nil {
		return "", fmt.Errorf("read built asset %q (run `make assets`?): %w", name, err)
	}
	return string(b), nil
}

// loadStyleCSS returns the stylesheet for a page, ready to inline or write.
// A dev override or operator custom layout may ship its own .css, which is used
// verbatim (comment-stripped, matching legacy behaviour); otherwise the
// Vite-built, pre-minified asset is returned.
func loadStyleCSS(layout, name string) (string, error) {
	if devTemplateDir != "" {
		if b, err := os.ReadFile(filepath.Join(devTemplateDir, layout, name)); err == nil {
			return utils.StripCSSComments(string(b)), nil
		}
	}
	if layoutsDir != "" {
		if b, err := os.ReadFile(filepath.Join(layoutsDir, layout, name)); err == nil {
			return utils.StripCSSComments(string(b)), nil
		}
	}
	return loadBuiltAsset(name)
}

// layoutsDir, when non-empty, is an on-disk directory of operator-supplied
// layouts (one subdirectory per layout). A profile can ship its own template
// pack there; any page it does not override falls back to the embedded default.
var layoutsDir string

// SetLayoutsDir registers an on-disk directory holding custom layouts. Pass an
// empty string to use only the embedded layouts.
func SetLayoutsDir(dir string) {
	layoutsDir = dir
}

// OnDiskLayoutExists reports whether dir/<layout> exists under the configured
// layouts directory. Used to validate profile-selected layouts that are not
// compiled into the binary.
func OnDiskLayoutExists(layout string) bool {
	if layoutsDir == "" || layout == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(layoutsDir, layout))
	return err == nil && info.IsDir()
}

// loadNotificationTemplate reads template file name belonging to the given layout.
// Resolution order: the dev override dir (if set), then the on-disk layouts dir,
// then the embedded layout, then the embedded default layout. The cascading
// fallback lets a custom on-disk layout override only some pages and inherit the
// rest from the default pack.
func loadNotificationTemplate(layout, name string) (string, error) {
	if devTemplateDir != "" {
		path := filepath.Join(devTemplateDir, layout, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		return string(b), nil
	}

	// On-disk custom layout (operator-supplied), if present for this page.
	if layoutsDir != "" {
		path := filepath.Join(layoutsDir, layout, name)
		if b, err := os.ReadFile(path); err == nil {
			return string(b), nil
		}
	}

	// Embedded layout for this page, then the embedded default as a last resort.
	if b, err := notificationTemplateFS.ReadFile("templates/" + layout + "/" + name); err == nil {
		return string(b), nil
	}
	b, err := notificationTemplateFS.ReadFile("templates/" + digestlayouts.Default() + "/" + name)
	if err != nil {
		return "", fmt.Errorf("no template %q for layout %q (and no default fallback): %w", name, layout, err)
	}
	return string(b), nil
}
