package scrapers

import (
	"sync"
)

var (
	sharedScraper     *AnonymizedScraper
	sharedScraperOnce sync.Once
	sharedScraperMu   sync.Mutex
)

// GetSharedAnonymizedScraper returns the process-wide AnonymizedScraper, which owns
// the reused Playwright browser/context and the User-Agent pool. Per-call colly
// collectors (see ScrapeContent) carry no shared state, so this needs no per-domain
// configuration.
func GetSharedAnonymizedScraper() *AnonymizedScraper {
	sharedScraperOnce.Do(func() {
		sharedScraper = NewAnonymizedScraper()
	})
	return sharedScraper
}

// CloseSharedPlaywright releases resources used by the shared Playwright instance
func CloseSharedPlaywright() {
	sharedScraperMu.Lock()
	defer sharedScraperMu.Unlock()

	if sharedScraper != nil {
		sharedScraper.ClosePlaywright()
	}
}
