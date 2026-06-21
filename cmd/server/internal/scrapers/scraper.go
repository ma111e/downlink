package scrapers

import (
	"github.com/ma111e/downlink/pkg/models"
)

// Scraper defines the interface for feed scrapers
type Scraper interface {
	// Fetch returns the feed's items plus the raw HTTP response it parsed them
	// from, so the bytes stay inspectable (it is non-nil even when parsing fails,
	// and nil only when nothing was fetched, e.g. a network-level error).
	Fetch(url string, params map[string]any) ([]models.FeedItem, *RawResponse, error)
	ScrapeContent(url string, params map[string]any) (string, error)
}

// HeadersFromParams extracts custom HTTP headers stored under params["headers"].
// Values may arrive as map[string]string (in-process) or map[string]any (after a
// JSON/protobuf round-trip), so both shapes are handled. Returns nil when absent.
func HeadersFromParams(params map[string]any) map[string]string {
	raw, ok := params["headers"]
	if !ok || raw == nil {
		return nil
	}

	out := map[string]string{}
	switch m := raw.(type) {
	case map[string]string:
		for k, v := range m {
			if v != "" {
				out[k] = v
			}
		}
	case map[string]any:
		for k, v := range m {
			if s, ok := v.(string); ok && s != "" {
				out[k] = s
			}
		}
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
