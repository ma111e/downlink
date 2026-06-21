package scrapers

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// commonFeedPaths are conventional feed locations probed relative to a site's
// root when in-page discovery turns up nothing. Each is validated before being
// surfaced, so a path that 404s or returns HTML is silently dropped.
var commonFeedPaths = []string{
	"/feed",
	"/feed.xml",
	"/rss",
	"/rss.xml",
	"/atom.xml",
	"/index.xml",
	"/feeds/posts/default",
	"/?feed=rss2",
}

// feedLinkTypes are the <link rel="alternate"> MIME types that denote a feed.
var feedLinkTypes = map[string]bool{
	"application/rss+xml":   true,
	"application/atom+xml":  true,
	"application/feed+json": true,
	"application/json":      true,
}

// maxValidatedCandidates caps how many candidate URLs validateFeedCandidates will
// fetch, so probing a hostile page can't fan out into dozens of requests.
const maxValidatedCandidates = 8

// discoverFeedLinks scans an HTML body for candidate feed URLs using three
// strategies, in descending confidence: <link rel="alternate"> autodiscovery,
// then anchors whose href/text look feed-ish, then conventional common paths.
// All hrefs are resolved against baseURL and the result is de-duped while
// preserving rank order. Candidates are unvalidated; callers should validate.
func discoverFeedLinks(body []byte, baseURL string) []string {
	base, _ := url.Parse(baseURL)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return commonPaths(baseURL)
	}

	var ranked []string
	seen := map[string]bool{}
	add := func(raw string) {
		abs := resolveRef(base, raw)
		if abs == "" || seen[abs] {
			return
		}
		seen[abs] = true
		ranked = append(ranked, abs)
	}

	// 1. <link rel="alternate" type="application/rss+xml"> autodiscovery.
	doc.Find("link[rel~=alternate]").Each(func(_ int, s *goquery.Selection) {
		typ := strings.ToLower(strings.TrimSpace(s.AttrOr("type", "")))
		href := s.AttrOr("href", "")
		if href != "" && feedLinkTypes[typ] {
			add(href)
		}
	})

	// 2. Anchors whose href or text look like a feed.
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href := s.AttrOr("href", "")
		if href != "" && (looksLikeFeed(href) || looksLikeFeed(s.Text())) {
			add(href)
		}
	})

	// 3. Conventional common paths (lowest confidence, validated downstream).
	for _, p := range commonPaths(baseURL) {
		if !seen[p] {
			seen[p] = true
			ranked = append(ranked, p)
		}
	}

	return ranked
}

// feedKeywords are matched at word-ish boundaries so "rss"/"atom"/"feed" hit but
// "breakfast" does not. A trailing feed extension also counts (see looksLikeFeed).
var feedKeywords = []string{"rss", "atom", "feed"}

// looksLikeFeed reports whether a href or link text resembles a feed reference:
// a feed file extension, or a feed keyword bounded by non-letters.
func looksLikeFeed(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	// Strip query/fragment for extension checks.
	path := low
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	for _, ext := range []string{".xml", ".rss", ".atom"} {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	for _, kw := range feedKeywords {
		if hasBoundedToken(low, kw) {
			return true
		}
	}
	return false
}

// hasBoundedToken reports whether token appears in s delimited by non-letter
// characters on both sides, so "/feed/" matches but "breakfast" does not.
func hasBoundedToken(s, token string) bool {
	from := 0
	for {
		i := strings.Index(s[from:], token)
		if i < 0 {
			return false
		}
		i += from
		beforeOK := i == 0 || !isLetter(s[i-1])
		afterIdx := i + len(token)
		afterOK := afterIdx >= len(s) || !isLetter(s[afterIdx])
		if beforeOK && afterOK {
			return true
		}
		from = i + 1
	}
}

func isLetter(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// commonPaths builds conventional feed candidate URLs relative to a base URL's
// site root, plus a couple relative to its current directory.
func commonPaths(baseURL string) []string {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	push := func(ref string) {
		if abs := resolveRef(base, ref); abs != "" && !seen[abs] {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	for _, p := range commonFeedPaths {
		push(p)
	}
	// Path-relative variants (e.g. a section that has its own feed).
	if p := strings.TrimRight(base.Path, "/"); p != "" {
		push(p + "/feed")
		push(p + "/rss.xml")
	}
	return out
}

// resolveRef resolves a possibly-relative href against base and returns the
// absolute URL string, or "" when it can't be parsed.
func resolveRef(base *url.URL, ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	if base != nil {
		return base.ResolveReference(u).String()
	}
	if u.IsAbs() {
		return u.String()
	}
	return ""
}

// validateFeedCandidates keeps only the candidates that actually fetch and parse
// as a feed (rss/atom/json-feed), preserving input order. It fetches at most
// maxValidatedCandidates URLs to bound the work, and skips the original URL when
// passed (the caller already knows it is HTML).
func validateFeedCandidates(cands []string, headers map[string]string, skip string) []string {
	var out []string
	checked := 0
	for _, c := range cands {
		if c == skip {
			continue
		}
		if checked >= maxValidatedCandidates {
			break
		}
		checked++
		raw, err := FetchRaw(c, headers)
		if err != nil || raw.Status >= 400 {
			continue
		}
		switch guessFeedType(raw.Body) {
		case "rss", "atom", "json-feed":
			out = append(out, raw.FinalURL)
		}
	}
	return dedupe(out)
}

// dedupe removes duplicate strings while preserving first-seen order.
func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
