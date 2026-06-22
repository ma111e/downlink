// Package devserver runs a local HTTP preview of the digest HTML templates with
// browser live-reload. It renders the digest/swipe/archive views from a fixed
// digest, re-reading templates from disk on every request, and pushes a reload to
// connected browsers whenever a *.tmpl file under the templates directory changes.
package devserver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"

	"github.com/ma111e/downlink/cmd/server/internal/notification"
	"github.com/ma111e/downlink/pkg/models"
)

// Options configures the dev preview server.
type Options struct {
	Addr         string          // listen address, e.g. ":8099"
	TemplatesDir string          // directory holding *.tmpl files, watched for changes
	OpenBrowser  bool            // open the default browser at startup
	Digests      []models.Digest // digests listed in the archive and served individually
	Theme        string          // template layout name; empty = default
}

// Run starts the preview server and blocks until the process is interrupted or
// the HTTP server fails.
func Run(opts Options) error {
	notification.SetTemplateDir(opts.TemplatesDir)

	hub := newReloadHub()

	watcher, err := startWatcher(opts.TemplatesDir, hub)
	if err != nil {
		return fmt.Errorf("watch templates: %w", err)
	}
	defer watcher.Close()

	// Digests created in the same minute share a filename; keep one per filename
	// (the first, i.e. newest) so route registration and the manifest stay in sync
	// and ServeMux doesn't panic on a duplicate pattern.
	digests := dedupeByFilename(opts.Digests)

	mux := http.NewServeMux()

	// The archive index is the landing page, mirroring the published site where
	// index.html is the archive shell at the root. Served at both / and
	// /index.html so links to either resolve.
	archiveIndex := func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		serveHTML(w, func() ([]byte, error) { return notification.RenderDigestIndex(opts.Theme, "") })
	}
	mux.HandleFunc("/", archiveIndex)
	mux.HandleFunc("/index.html", archiveIndex)

	// One pair of routes per digest, at the real filenames the archive index and
	// the digest pages link to (base "", so links are bare filenames).
	for _, d := range digests {
		d := d
		digestFilename := notification.DigestHTMLFilename(d)
		mux.HandleFunc("/"+digestFilename, func(w http.ResponseWriter, r *http.Request) {
			serveHTML(w, func() ([]byte, error) {
				return notification.RenderDigestHTML(d, opts.Theme, "")
			})
		})
		mux.HandleFunc("/"+notification.SwipeHTMLFilename(d), func(w http.ResponseWriter, r *http.Request) {
			serveHTML(w, func() ([]byte, error) {
				return notification.RenderSwipeHTML(d, digestFilename, opts.Theme, "")
			})
		})
	}

	// Stub manifest listing every preview digest so the archive shell renders.
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m := notification.Manifest{
			SourceRepo:  "downlink-dev",
			GeneratedAt: time.Now().UTC().Format("2006-01-02 15:04 UTC"),
		}
		for _, d := range digests {
			m.Upsert(notification.ManifestEntryFromDigest(d))
		}
		b, err := json.MarshalIndent(m, "", "  ")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	})

	mux.HandleFunc("/__livereload", hub.handleSSE)

	url := "http://localhost" + normalizeAddrForURL(opts.Addr)
	log.WithField("addr", opts.Addr).Info("digest dev server listening")
	fmt.Printf("\n  archive index:   %s  (%d digest(s))\n", url, len(digests))
	if len(digests) == 1 {
		d := digests[0]
		fmt.Printf("  digest:          %s/%s\n  swipe:           %s/%s\n", url, notification.DigestHTMLFilename(d), url, notification.SwipeHTMLFilename(d))
	}
	fmt.Println()

	if opts.OpenBrowser {
		openBrowser(url)
	}

	return http.ListenAndServe(opts.Addr, mux)
}

// dedupeByFilename returns digests with duplicate digest filenames removed,
// keeping the first occurrence of each (the newest, given a newest-first input).
func dedupeByFilename(in []models.Digest) []models.Digest {
	seen := make(map[string]bool, len(in))
	out := make([]models.Digest, 0, len(in))
	for _, d := range in {
		name := notification.DigestHTMLFilename(d)
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, d)
	}
	return out
}

// serveHTML renders via render and writes the result with the live-reload snippet
// injected. Render errors are returned to the browser so they're visible on reload.
func serveHTML(w http.ResponseWriter, render func() ([]byte, error)) {
	html, err := render()
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "render error:\n\n%v", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(injectReload(html))
}

const reloadSnippet = `<script>
(function () {
  var connected = false;
  function connect() {
    var es = new EventSource("/__livereload");
    es.addEventListener("open", function () {
      if (connected) { location.reload(); } // server came back (e.g. air restart)
      connected = true;
    });
    es.addEventListener("message", function () { location.reload(); });
    es.addEventListener("error", function () { connected = false; }); // browser auto-reconnects
  }
  connect();
})();
</script>`

// injectReload inserts the live-reload script just before </body>, or appends it
// when there is no body tag.
func injectReload(html []byte) []byte {
	marker := []byte("</body>")
	if i := bytes.LastIndex(html, marker); i >= 0 {
		var out bytes.Buffer
		out.Write(html[:i])
		out.WriteString(reloadSnippet)
		out.Write(html[i:])
		return out.Bytes()
	}
	return append(html, []byte(reloadSnippet)...)
}

// reloadHub fans a single reload signal out to every connected SSE client.
type reloadHub struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func newReloadHub() *reloadHub {
	return &reloadHub{clients: make(map[chan struct{}]struct{})}
}

func (h *reloadHub) add() chan struct{} {
	ch := make(chan struct{}, 1)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *reloadHub) remove(ch chan struct{}) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

func (h *reloadHub) broadcast() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- struct{}{}:
		default: // a reload is already queued for this client
		}
	}
}

func (h *reloadHub) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := h.add()
	defer h.remove(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ch:
			fmt.Fprint(w, "data: reload\n\n")
			flusher.Flush()
		}
	}
}

// startWatcher watches dir (and its layout subdirectories) for *.tmpl changes and
// triggers a debounced reload. fsnotify is not recursive, so each directory holding
// templates is added explicitly.
func startWatcher(dir string, hub *reloadHub) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("add %s: %w", dir, err)
	}
	// Templates live one level down in per-layout subdirectories; watch each.
	entries, err := os.ReadDir(dir)
	if err != nil {
		watcher.Close()
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		if err := watcher.Add(sub); err != nil {
			watcher.Close()
			return nil, fmt.Errorf("add %s: %w", sub, err)
		}
	}

	go func() {
		var timer *time.Timer
		fire := func() { hub.broadcast() }
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".tmpl") {
					continue
				}
				if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
					continue
				}
				log.WithField("file", filepath.Base(event.Name)).Debug("template changed, reloading")
				// Debounce: editors often emit several events per save.
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(120*time.Millisecond, fire)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.WithError(err).Warn("template watcher error")
			}
		}
	}()

	return watcher, nil
}

// normalizeAddrForURL turns a listen address into a URL suffix, defaulting a bare
// ":port" to that port and a bare host to :80.
func normalizeAddrForURL(addr string) string {
	if addr == "" {
		return ""
	}
	if strings.HasPrefix(addr, ":") {
		return addr
	}
	if !strings.Contains(addr, ":") {
		return ":" + addr
	}
	// host:port -> keep as-is but the host portion is already localhost in the URL.
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return addr
}

// openBrowser best-effort opens url in the default browser.
func openBrowser(url string) {
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		log.WithError(err).Debug("could not open browser")
	}
}
