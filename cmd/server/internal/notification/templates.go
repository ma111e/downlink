package notification

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed templates/*.tmpl
var notificationTemplateFS embed.FS

// devTemplateDir, when non-empty, makes loadNotificationTemplate read templates
// from disk instead of the embedded FS. Used by the digest dev server so template
// edits show up on the next render with no recompile. Set via SetTemplateDir.
var devTemplateDir string

// SetTemplateDir switches template loading to read from dir on disk. Pass an empty
// string to fall back to the embedded templates. Intended for the dev preview server.
func SetTemplateDir(dir string) {
	devTemplateDir = dir
}

func loadNotificationTemplate(name string) (string, error) {
	if devTemplateDir != "" {
		path := filepath.Join(devTemplateDir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		return string(b), nil
	}

	path := "templates/" + name
	b, err := notificationTemplateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
