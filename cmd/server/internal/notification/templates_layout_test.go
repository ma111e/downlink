package notification

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadNotificationTemplateOnDiskFallback verifies that an on-disk custom
// layout overrides only the pages it provides, with every other page cascading
// to the embedded default pack.
func TestLoadNotificationTemplateOnDiskFallback(t *testing.T) {
	dir := t.TempDir()
	customDigest := "<html>CUSTOM DIGEST PACK</html>"
	if err := os.MkdirAll(filepath.Join(dir, "press"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "press", "digest.html.tmpl"), []byte(customDigest), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	SetLayoutsDir(dir)
	t.Cleanup(func() { SetLayoutsDir("") })

	if !OnDiskLayoutExists("press") {
		t.Fatal("OnDiskLayoutExists(press) = false, want true")
	}
	if OnDiskLayoutExists("nope") {
		t.Error("OnDiskLayoutExists(nope) = true, want false")
	}

	// The overridden page comes from disk.
	got, err := loadNotificationTemplate("press", "digest.html.tmpl")
	if err != nil {
		t.Fatalf("loadNotificationTemplate(digest): %v", err)
	}
	if got != customDigest {
		t.Errorf("digest template = %q, want custom on-disk content", got)
	}

	// A page the custom layout does NOT provide falls back to the embedded default.
	idx, err := loadNotificationTemplate("press", "archive-index.html.tmpl")
	if err != nil {
		t.Fatalf("loadNotificationTemplate(archive-index): %v", err)
	}
	if idx == "" || idx == customDigest {
		t.Errorf("archive-index did not fall back to embedded default")
	}
	if !strings.Contains(idx, "<") {
		t.Errorf("archive-index fallback does not look like HTML: %.40q", idx)
	}
}
