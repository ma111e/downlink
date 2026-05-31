package trace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// reset returns the package to a disabled, clean state between tests.
func reset() {
	enabled = false
	baseDir = ""
	seq.Store(0)
}

func TestDisabledIsNoOp(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := Init(dir, false); err != nil {
		t.Fatalf("Init(disabled) returned error: %v", err)
	}
	if Enabled() {
		t.Fatal("Enabled() should be false when initialized with on=false")
	}
	// Writers must be safe no-ops while disabled.
	LLM("x", "p", "r", time.Second, nil, nil)
	HTTP("GET", "http://e/x", 200, "text/xml", []byte("a"), time.Second)
	Scrape("a", "u", "ok", "<html>")

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("expected no files written while disabled, got %d", len(entries))
	}
}

func TestHTTPPreservesRawBytes(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := Init(dir, true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !Enabled() {
		t.Fatal("Enabled() should be true")
	}

	// Non-UTF-8 payload (0xff 0xfe ...) must round-trip byte-for-byte.
	raw := []byte{0x3c, 0x3f, 0x78, 0x6d, 0x6c, 0xff, 0xfe, 0x00}
	HTTP("GET", "https://feeds.example.com/rss", 200, "application/rss+xml; charset=iso-8859-1", raw, 5*time.Millisecond)

	xmls := globOne(t, filepath.Join(dir, "fetch", "*.xml"))
	got, err := os.ReadFile(xmls)
	if err != nil {
		t.Fatalf("read body file: %v", err)
	}
	if string(got) != string(raw) {
		t.Fatalf("raw body not preserved: got % x want % x", got, raw)
	}
	// Sidecar meta should exist.
	globOne(t, filepath.Join(dir, "fetch", "*.meta.json"))
}

func TestLLMAndScrapeWriteFiles(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := Init(dir, true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	LLM("analyze:task=categorize", "the prompt", "the response", 10*time.Millisecond, nil, nil)
	Scrape("article-123", "https://x/y", "ok", "<html>body</html>")

	globOne(t, filepath.Join(dir, "llm", "*.json"))
	globOne(t, filepath.Join(dir, "scrape", "*.html"))
	globOne(t, filepath.Join(dir, "scrape", "*.meta.json"))
}

func TestInitPreCreatesSubdirs(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := Init(dir, true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for _, kind := range []string{"llm", "fetch", "scrape", "content"} {
		info, err := os.Stat(filepath.Join(dir, kind))
		if err != nil || !info.IsDir() {
			t.Fatalf("expected subdir %q to exist after Init, err=%v", kind, err)
		}
	}
}

func TestContentPreservesInvalidUTF8(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := Init(dir, true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// A mostly-text payload with an invalid UTF-8 byte (0xff) in the middle.
	bad := "before\xffafter"
	Content("article-9", "https://x/y", "invalid-utf8", bad)

	f := globOne(t, filepath.Join(dir, "content", "*-invalid-utf8.txt"))
	got, err := os.ReadFile(f)
	if err != nil {
		t.Fatalf("read content file: %v", err)
	}
	if string(got) != bad {
		t.Fatalf("content not preserved byte-for-byte: got % x want % x", got, bad)
	}
	globOne(t, filepath.Join(dir, "content", "*-invalid-utf8.meta.json"))
}

func TestDefaultDirUnderTemp(t *testing.T) {
	reset()
	if err := Init("", true); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer os.RemoveAll(baseDir)
	if filepath.Dir(baseDir) != os.TempDir() {
		t.Fatalf("default trace dir %q not under temp dir %q", baseDir, os.TempDir())
	}
}

func globOne(t *testing.T, pattern string) string {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 file for %s, got %d", pattern, len(matches))
	}
	return matches[0]
}
