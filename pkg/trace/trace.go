// Package trace is an opt-in, content-level debug tracer. When enabled (the
// server runs at the `trace` log level) it dumps the raw bytes flowing through
// the system (LLM prompt/response chains, raw HTTP feed-fetch bodies, and
// solimen-scraped HTML) to discrete files on disk so issues like non-UTF-8
// feed responses or a misbehaving prompt chain can be inspected directly.
//
// It is stdlib-only on purpose: keeping it free of project imports lets every
// layer (llmgateway, scrapers, manager) call into it without import cycles.
//
// Every exported writer is a no-op when tracing is disabled and never returns
// an error to its caller; a failed trace write is logged and ignored so the
// debug facility can never break the main flow.
package trace

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

var (
	enabled bool
	baseDir string
	seq     atomic.Uint64
)

// Init resolves the trace root and, when enabled, creates it and records its
// location. dir overrides the location; when empty a dedicated per-run folder
// under the OS temp dir is used (e.g. /tmp/downlink-trace-<timestamp>). When
// enabled is false this is a no-op and Enabled() stays false.
func Init(dir string, on bool) error {
	if !on {
		return nil
	}
	root := strings.TrimSpace(dir)
	if root == "" {
		root = filepath.Join(os.TempDir(), "downlink-trace-"+time.Now().Format("20060102-150405"))
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		// Tracing is best-effort: warn and stay disabled rather than fail startup.
		log.WithError(err).WithField("dir", root).Warn("trace: failed to create trace dir; tracing disabled")
		return err
	}
	// Pre-create the per-type subfolders so the structure is visible the moment
	// tracing is on, regardless of which code paths actually fire.
	for _, kind := range []string{"llm", "fetch", "scrape", "content"} {
		if err := os.MkdirAll(filepath.Join(root, kind), 0o755); err != nil {
			log.WithError(err).WithField("dir", filepath.Join(root, kind)).Warn("trace: failed to create trace subdir")
		}
	}
	baseDir = root
	enabled = true
	log.WithField("dir", root).Info("trace enabled: writing prompt/response and feed/scrape content")
	return nil
}

// Enabled reports whether tracing is active. Call sites that must build a
// payload (e.g. read an HTTP body) should guard that work with this.
func Enabled() bool { return enabled }

// prefix returns a chronologically-sortable, collision-free filename prefix.
func prefix() string {
	return fmt.Sprintf("%s-%06d", time.Now().Format("150405.000000000"), seq.Add(1))
}

// subPath ensures <baseDir>/<kind> exists and returns <baseDir>/<kind>/<name>.
func subPath(kind, name string) (string, bool) {
	dir := filepath.Join(baseDir, kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.WithError(err).WithField("dir", dir).Warn("trace: failed to create subdir")
		return "", false
	}
	return filepath.Join(dir, name), true
}

func writeFile(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.WithError(err).WithField("path", path).Warn("trace: failed to write trace file")
	}
}

func writeJSON(path string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.WithError(err).WithField("path", path).Warn("trace: failed to marshal trace record")
		return
	}
	writeFile(path, data)
}

// sanitize makes s safe for a filename segment.
func sanitize(s string) string {
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
	if len(s) > 80 {
		s = s[:80]
	}
	if s == "" {
		s = "none"
	}
	return s
}

// LLM records a single prompt/response chain as one self-contained JSON file.
func LLM(label, prompt, response string, dur time.Duration, callErr error, meta map[string]string) {
	if !enabled {
		return
	}
	path, ok := subPath("llm", prefix()+"-"+sanitize(label)+".json")
	if !ok {
		return
	}
	rec := map[string]any{
		"time":        time.Now().Format(time.RFC3339Nano),
		"label":       label,
		"duration_ms": dur.Milliseconds(),
		"prompt":      prompt,
		"response":    response,
	}
	if callErr != nil {
		rec["error"] = callErr.Error()
	}
	if len(meta) > 0 {
		rec["meta"] = meta
	}
	writeJSON(path, rec)
}

// HTTP records a raw HTTP response body as its own file (exact bytes, so
// non-UTF-8 payloads stay inspectable) alongside a small metadata sidecar.
func HTTP(method, rawURL string, status int, contentType string, body []byte, dur time.Duration) {
	if !enabled {
		return
	}
	host := "unknown"
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		host = u.Host
	}
	base := fmt.Sprintf("%s-%s-%d", prefix(), sanitize(host), status)

	if path, ok := subPath("fetch", base+extForContentType(contentType)); ok {
		writeFile(path, body)
	}
	if path, ok := subPath("fetch", base+".meta.json"); ok {
		writeJSON(path, map[string]any{
			"time":         time.Now().Format(time.RFC3339Nano),
			"method":       method,
			"url":          rawURL,
			"status":       status,
			"content_type": contentType,
			"bytes":        len(body),
			"duration_ms":  dur.Milliseconds(),
		})
	}
}

// SolimenRequest records the JSON query body POSTed to the solimen scrape
// service, so the exact request can be compared against its result.
func SolimenRequest(articleID, rawURL string, payload []byte) {
	if !enabled {
		return
	}
	if path, ok := subPath("scrape", prefix()+"-"+sanitize(articleID)+".request.json"); ok {
		writeFile(path, payload)
	}
}

// Scrape records the raw HTML returned by a browser scrape (solimen) plus a
// metadata sidecar.
func Scrape(articleID, rawURL, state, html string) {
	if !enabled {
		return
	}
	base := prefix() + "-" + sanitize(articleID)
	if path, ok := subPath("scrape", base+".html"); ok {
		writeFile(path, []byte(html))
	}
	if path, ok := subPath("scrape", base+".meta.json"); ok {
		writeJSON(path, map[string]any{
			"time":       time.Now().Format(time.RFC3339Nano),
			"article_id": articleID,
			"url":        rawURL,
			"state":      state,
			"bytes":      len(html),
		})
	}
}

// Content records article content that was rejected/notable for a given reason
// (e.g. "invalid-utf8") as its own byte-exact file plus a metadata sidecar, so
// the offending bytes, which would otherwise be dropped, stay inspectable.
func Content(articleID, rawURL, reason, content string) {
	if !enabled {
		return
	}
	base := fmt.Sprintf("%s-%s-%s", prefix(), sanitize(articleID), sanitize(reason))
	if path, ok := subPath("content", base+".txt"); ok {
		writeFile(path, []byte(content))
	}
	if path, ok := subPath("content", base+".meta.json"); ok {
		writeJSON(path, map[string]any{
			"time":       time.Now().Format(time.RFC3339Nano),
			"article_id": articleID,
			"url":        rawURL,
			"reason":     reason,
			"bytes":      len(content),
			"valid_utf8": utf8.ValidString(content),
		})
	}
}

// SaveDiagnostic writes a raw feed body to disk regardless of whether tracing is
// enabled, and returns the absolute path it wrote (empty on failure). It backs the
// on-demand `feeds diagnose` command and the parse-failure breadcrumb: those need
// the offending bytes even when the server is not running at trace log level.
//
// When tracing is on the file lands under the active trace dir's fetch/ folder so
// it sits alongside the rest of the run; otherwise it goes to a stable, lazily
// created os.TempDir()/downlink-diagnose folder. Like every other writer here it
// never returns an error to its caller.
func SaveDiagnostic(host string, status int, contentType string, body []byte) string {
	var dir string
	if enabled && baseDir != "" {
		dir = filepath.Join(baseDir, "fetch")
	} else {
		dir = filepath.Join(os.TempDir(), "downlink-diagnose")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.WithError(err).WithField("dir", dir).Warn("trace: failed to create diagnose dir")
		return ""
	}
	name := fmt.Sprintf("%s-%s-%d%s", prefix(), sanitize(host), status, extForContentType(contentType))
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		log.WithError(err).WithField("path", path).Warn("trace: failed to write diagnose file")
		return ""
	}
	return path
}

func extForContentType(ct string) string {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "xml"):
		return ".xml"
	case strings.Contains(ct, "html"):
		return ".html"
	case strings.Contains(ct, "json"):
		return ".json"
	case strings.Contains(ct, "text/"):
		return ".txt"
	default:
		return ".bin"
	}
}
