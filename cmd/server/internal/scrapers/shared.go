package scrapers

import (
	"sync"
)

var (
	sharedScraper     *AnonymizedScraper
	sharedScraperOnce sync.Once
	sharedScraperMu   sync.Mutex
)

// GetSharedAnonymizedScraper returns a shared AnonymizedScraper instance.
// If domain is provided, it updates the allowed domains.
func GetSharedAnonymizedScraper(domain string) *AnonymizedScraper {
	sharedScraperOnce.Do(func() {
		// Initialize with an empty domain (no restrictions)
		sharedScraper = NewAnonymizedScraper("")
	})

	// If a specific domain is provided, ensure it's allowed
	if domain != "" {
		sharedScraperMu.Lock()
		defer sharedScraperMu.Unlock()
		
		// Update the collector's allowed domains if needed
		// This is safe because we're using a mutex to protect access
		sharedScraper.Collector.AllowedDomains = append(
			sharedScraper.Collector.AllowedDomains, 
			domain,
		)
	}

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
