package scrapers

import (
	"sync"
)

var (
	sharedScraper     *AnonymizedScraper
	sharedScraperOnce sync.Once
)

// GetSharedAnonymizedScraper returns the process-wide AnonymizedScraper, which owns
// the User-Agent pool. It holds no browser or collector state — ScrapeContent builds a
// per-call colly collector and ScrapeContentDynamic opens a per-call CDP connection to
// Lightpanda — so this needs no configuration and nothing to close.
func GetSharedAnonymizedScraper() *AnonymizedScraper {
	sharedScraperOnce.Do(func() {
		sharedScraper = NewAnonymizedScraper()
	})
	return sharedScraper
}
