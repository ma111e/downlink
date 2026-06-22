package notification

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ma111e/downlink/pkg/digestlayouts"
)

//go:embed templates/*/*.tmpl
var notificationTemplateFS embed.FS

//go:embed static/*
var staticFS embed.FS

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
