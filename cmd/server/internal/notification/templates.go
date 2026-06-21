package notification

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
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

// loadNotificationTemplate reads template file name belonging to the given layout
// (a subdirectory of templates/). Each layout holds a full set of page templates.
func loadNotificationTemplate(layout, name string) (string, error) {
	if devTemplateDir != "" {
		path := filepath.Join(devTemplateDir, layout, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		return string(b), nil
	}

	path := "templates/" + layout + "/" + name
	b, err := notificationTemplateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
