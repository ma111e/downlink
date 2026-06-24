package scrapers

import (
	"net/url"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	// minLinkGroup is the smallest number of distinct post URLs a selector must
	// match to be treated as a repeating post list (not a one-off link).
	minLinkGroup = 3
	// linkSampleHrefs caps how many resolved URLs a candidate carries as samples.
	linkSampleHrefs = 5
	// linkBlockMaxDepth bounds how far up from an anchor we look for the repeating
	// block selector and the block's date element.
	linkBlockMaxDepth = 4
	// dateBlockCap bounds how many anchors per group we probe for a date selector.
	dateBlockCap = 12
)

// LinkListCandidate is a ranked guess at the repeating post-link structure on an
// HTML index page. Mirrors models.LinkListCandidate (the models copy is what
// crosses the service boundary; this one is produced here, over the DOM).
type LinkListCandidate struct {
	LinksSelector string   `json:"links_selector"`
	Count         int      `json:"count"`
	SampleHrefs   []string `json:"sample_hrefs"`
	DateSelector  string   `json:"date_selector,omitempty"`
	DateSample    string   `json:"date_sample,omitempty"`
	URLFilter     string   `json:"url_filter,omitempty"`
}

// linkGroup accumulates stats for one candidate links_selector while scanning the
// page's anchors.
type linkGroup struct {
	hrefs   map[string]bool      // distinct on-site, non-fragment resolved URLs
	samples []string             // first few resolved URLs (insertion order)
	anchors []*goquery.Selection // a few anchors, for date-selector detection
	offsite int                  // anchors whose href left the page's host
	total   int                  // every anchor the selector covered
}

// SuggestLinkSelectors walks an index-page DOM and returns ranked guesses at the
// selector scoping its repeating post links, plus the date_selector and url_filter
// implied by that repeating block. It groups anchors by the selector that would
// match them — the anchor's own tag.class and its nearest classed ancestor block
// (e.g. "article.card a") — keeps groups with enough distinct on-site URLs, and
// ranks them by URL count weighted by path cohesion and penalized by off-site
// links (so nav/footer/social rows sink). Pure over the DOM, so it is unit-testable.
func SuggestLinkSelectors(dom *goquery.Selection, base *url.URL, max int) []LinkListCandidate {
	if dom == nil {
		return nil
	}
	if max <= 0 {
		max = 5
	}

	groups := map[string]*linkGroup{}
	dom.Find("a[href]").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		href = strings.TrimSpace(href)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
			return
		}
		resolved := resolveHref(base, href)
		onsite := sameHost(base, resolved)

		for _, selr := range anchorSelectors(a) {
			g := groups[selr]
			if g == nil {
				g = &linkGroup{hrefs: map[string]bool{}}
				groups[selr] = g
			}
			g.total++
			if !onsite {
				g.offsite++
				continue
			}
			if g.hrefs[resolved] {
				continue
			}
			g.hrefs[resolved] = true
			if len(g.samples) < linkSampleHrefs {
				g.samples = append(g.samples, resolved)
			}
			if len(g.anchors) < dateBlockCap {
				g.anchors = append(g.anchors, a)
			}
		}
	})

	var cands []LinkListCandidate
	for selr, g := range groups {
		distinct := len(g.hrefs)
		if distinct < minLinkGroup {
			continue
		}
		filter, _ := commonPathSegment(g.hrefs)
		dateSel, dateSample := detectDateSelector(g.anchors)
		cands = append(cands, LinkListCandidate{
			LinksSelector: selr,
			Count:         distinct,
			SampleHrefs:   g.samples,
			DateSelector:  dateSel,
			DateSample:    dateSample,
			URLFilter:     filter,
		})
	}

	sort.Slice(cands, func(i, j int) bool {
		gi, gj := groups[cands[i].LinksSelector], groups[cands[j].LinksSelector]
		return linkScore(cands[i], gi) > linkScore(cands[j], gj)
	})
	if len(cands) > max {
		cands = cands[:max]
	}
	return cands
}

// linkScore ranks a candidate: more distinct posts is better, multiplied by how
// cohesive their paths are (a shared "/blog/" segment), and dragged down by the
// fraction of off-site links the selector also caught (nav/footer/social).
func linkScore(c LinkListCandidate, g *linkGroup) float64 {
	_, cohesion := commonPathSegment(g.hrefs)
	offRatio := 0.0
	if g.total > 0 {
		offRatio = float64(g.offsite) / float64(g.total)
	}
	return float64(c.Count) * cohesion * (1.0 - offRatio)
}

// anchorSelectors returns the candidate selectors that would match this anchor: its
// own tag.class (when it has one) and "<nearest classed ancestor> a" (so a list of
// class-less anchors inside repeating blocks is still groupable).
func anchorSelectors(a *goquery.Selection) []string {
	var out []string
	if own := elementSelector(a); own != "" && !strings.HasPrefix(own, "#") {
		out = append(out, own)
	}
	if block := nearestBlockSelector(a); block != "" {
		out = append(out, block+" a")
	}
	return out
}

// nearestBlockSelector walks up from an anchor and returns the first ancestor whose
// elementSelector is a reusable group selector (a tag.class or a bare semantic tag),
// skipping id selectors (unique per element, so they can't group siblings).
func nearestBlockSelector(a *goquery.Selection) string {
	node := a.Parent()
	for depth := 0; depth < linkBlockMaxDepth; depth++ {
		if node == nil || len(node.Nodes) == 0 {
			break
		}
		if sel := elementSelector(node); sel != "" && !strings.HasPrefix(sel, "#") {
			return sel
		}
		node = node.Parent()
	}
	return ""
}

// detectDateSelector probes a few of a group's anchors for the date element in
// their block and returns the relative selector that appears in the most blocks,
// plus one sample of its text. Returns "" when no block date is found.
func detectDateSelector(anchors []*goquery.Selection) (string, string) {
	counts := map[string]int{}
	samples := map[string]string{}
	for _, a := range anchors {
		node := a
		for depth := 0; depth < blockDateMaxDepth; depth++ {
			if node == nil || len(node.Nodes) == 0 {
				break
			}
			if selr, sample, ok := blockDateSelector(node); ok {
				counts[selr]++
				if samples[selr] == "" {
					samples[selr] = sample
				}
				break
			}
			node = node.Parent()
		}
	}
	best, bestN := "", 0
	for selr, n := range counts {
		if n > bestN {
			best, bestN = selr, n
		}
	}
	return best, samples[best]
}

// dateTagSet are the leaf elements blockDateSelector scans for a date, ordered so a
// semantic <time> is preferred.
var dateTagSet = "time, span, small, em, p, li, div"

// blockDateSelector finds the first leaf element under block whose text/attr parses
// as a date and returns a relative selector for it (e.g. "time" or "span.date").
func blockDateSelector(block *goquery.Selection) (string, string, bool) {
	var selr, sample string
	block.Find(dateTagSet).EachWithBreak(func(_ int, e *goquery.Selection) bool {
		if e.Children().Length() > 0 {
			return true // not a leaf; its date-bearing child is scanned separately
		}
		if _, ok := parseItemDate(e); !ok {
			return true
		}
		if s := relativeDateSelector(e); s != "" {
			selr = s
			sample = strings.TrimSpace(e.Text())
			return false
		}
		return true
	})
	return selr, sample, selr != ""
}

// relativeDateSelector builds a relative selector for a date element: the bare
// "time" tag when applicable, else its tag.class, else the bare tag.
func relativeDateSelector(e *goquery.Selection) string {
	if len(e.Nodes) == 0 {
		return ""
	}
	tag := e.Nodes[0].Data
	if tag == "time" {
		return "time"
	}
	if sel := elementSelector(e); sel != "" && !strings.HasPrefix(sel, "#") {
		return sel
	}
	return tag
}

// commonPathSegment returns the leading path segment shared by most URLs (as a
// "/segment/" url_filter) and the fraction of URLs that share it. Returns "", 0
// when no segment is common enough to be useful.
func commonPathSegment(hrefs map[string]bool) (string, float64) {
	if len(hrefs) == 0 {
		return "", 0
	}
	counts := map[string]int{}
	for h := range hrefs {
		if seg := firstPathSegment(h); seg != "" {
			counts[seg]++
		}
	}
	best, bestN := "", 0
	for seg, n := range counts {
		if n > bestN {
			best, bestN = seg, n
		}
	}
	cohesion := float64(bestN) / float64(len(hrefs))
	if best == "" || cohesion < 0.6 {
		return "", cohesion
	}
	return "/" + best + "/", cohesion
}

// firstPathSegment returns the first non-empty path segment of a URL ("" for root).
func firstPathSegment(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg != "" {
			return seg
		}
	}
	return ""
}

// resolveHref resolves a possibly-relative href against the page base.
func resolveHref(base *url.URL, href string) string {
	if base == nil {
		return href
	}
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

// sameHost reports whether a resolved URL stays on the page's host (base nil =>
// treat as on-site so host-less fixtures still work).
func sameHost(base *url.URL, resolved string) bool {
	if base == nil {
		return true
	}
	u, err := url.Parse(resolved)
	if err != nil {
		return false
	}
	return u.Host == "" || u.Host == base.Host
}
