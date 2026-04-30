package notification

import (
	"embed"
	"fmt"
)

//go:embed templates/*.tmpl
var notificationTemplateFS embed.FS

func loadNotificationTemplate(name string) (string, error) {
	path := "templates/" + name
	b, err := notificationTemplateFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(b), nil
}
