package scrapers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestScrapeContentConcurrent guards the shared-collector bug fix: ScrapeContent must
// build a fresh, request-scoped collector per call so that (a) the same URL can be
// scraped repeatedly (no colly dedup) and (b) many concurrent scrapes never overwrite
// each other's result via shared callbacks. Run under -race to catch the regression.
func TestScrapeContentConcurrent(t *testing.T) {
	// Each path echoes its own marker inside the body, so a cross-fired handler that
	// returned another request's DOM would be detected.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<html><body><div id=marker>%s</div></body></html>", strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer srv.Close()

	s := NewAnonymizedScraper()

	// Build the request set: 15 distinct paths plus 5 repeats of one path (to prove
	// re-scraping is allowed — the original "URL already visited" failure).
	var paths []string
	for i := 0; i < 15; i++ {
		paths = append(paths, fmt.Sprintf("a%02d", i))
	}
	for i := 0; i < 5; i++ {
		paths = append(paths, "repeat")
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(paths))
	for _, p := range paths {
		wg.Add(1)
		go func(want string) {
			defer wg.Done()
			dom, err := s.ScrapeContent(srv.URL+"/"+want, nil)
			if err != nil {
				errs <- fmt.Errorf("%s: scrape error: %w", want, err)
				return
			}
			if dom == nil {
				errs <- fmt.Errorf("%s: nil dom (dedup-skipped or no body)", want)
				return
			}
			got := strings.TrimSpace(dom.Find("#marker").Text())
			if got != want {
				errs <- fmt.Errorf("cross-fire: requested %q but got %q", want, got)
			}
		}(p)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
