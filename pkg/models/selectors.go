package models

// GetEffectiveSelectors returns the effective selectors to use for a feed
// It prioritizes feed-specific selectors, then falls back to config defaults, then to system defaults
func GetEffectiveSelectors(feed *Feed, configDefaults *Selectors) *Selectors {
	// Start with system defaults
	result := &Selectors{}

	// Override with config defaults if available
	if configDefaults != nil {
		if configDefaults.Article != "" {
			result.Article = configDefaults.Article
		}
		if configDefaults.Cutoff != "" {
			result.Cutoff = configDefaults.Cutoff
		}
		if configDefaults.Blacklist != "" {
			result.Blacklist = configDefaults.Blacklist
		}
	}

	// Finally, override with feed-specific selectors if available
	if selectors, ok := feed.Scraper["selectors"].(Selectors); ok {
		if selectors.Article != "" {
			result.Article = selectors.Article
		}
		if selectors.Cutoff != "" {
			result.Cutoff = selectors.Cutoff
		}
		if selectors.Blacklist != "" {
			result.Blacklist = selectors.Blacklist
		}
	}

	return result
}
