package scrapers

import (
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// minCandidateChars is the smallest text length an element must hold to be worth
// proposing as an article-content selector.
const minCandidateChars = 200

// SelectorCandidate is a ranked guess at the element that wraps an article body.
type SelectorCandidate struct {
	Selector    string  `json:"selector"`
	Chars       int     `json:"chars"`        // trimmed text length
	LinkDensity float64 `json:"link_density"` // anchor text ÷ total text (0..1); high = nav/menu
	Snippet     string  `json:"snippet"`      // first printable chars of the element's text
}

// SuggestSelectors walks a page DOM and returns the most likely article-content
// selectors, ranked by text length penalized by link density (so nav/menus/footers,
// which are mostly links, sink below prose). It is a deterministic, token-cheap
// alternative to feeding raw HTML to an LLM: the agent picks from measured candidates
// and confirms with test_selector. Pure over the DOM, so it is unit-testable.
func SuggestSelectors(dom *goquery.Selection, max int) []SelectorCandidate {
	if dom == nil {
		return nil
	}
	if max <= 0 {
		max = 8
	}

	seen := make(map[string]bool)
	var cands []SelectorCandidate

	dom.Find("*").Each(func(_ int, s *goquery.Selection) {
		sel := elementSelector(s)
		if sel == "" || seen[sel] {
			return
		}
		text := strings.TrimSpace(s.Text())
		n := len([]rune(text))
		if n < minCandidateChars {
			return
		}

		linkText := 0
		s.Find("a").Each(func(_ int, a *goquery.Selection) {
			linkText += len([]rune(strings.TrimSpace(a.Text())))
		})
		density := 0.0
		if n > 0 {
			density = float64(linkText) / float64(n)
		}
		if density > 1 {
			density = 1
		}

		seen[sel] = true
		cands = append(cands, SelectorCandidate{
			Selector:    sel,
			Chars:       n,
			LinkDensity: density,
			Snippet:     snippet([]byte(text), 120),
		})
	})

	sort.Slice(cands, func(i, j int) bool {
		return candScore(cands[i]) > candScore(cands[j])
	})
	if len(cands) > max {
		cands = cands[:max]
	}
	return cands
}

// candScore ranks a candidate: more text is better, links drag it down.
func candScore(c SelectorCandidate) float64 {
	return float64(c.Chars) * (1.0 - c.LinkDensity)
}

// elementSelector builds a stable, valid CSS selector for an element: "#id" when it
// has a usable id, else "tag.class1.class2" from up to two simple classes, else the
// bare tag for the semantic containers article/main. Returns "" for elements that
// can't yield a useful, valid selector (so we never emit something goquery can't parse).
func elementSelector(s *goquery.Selection) string {
	if len(s.Nodes) == 0 {
		return ""
	}
	tag := s.Nodes[0].Data
	switch tag {
	case "html", "head", "script", "style", "noscript", "svg", "path", "body":
		return ""
	}

	if id, ok := s.Attr("id"); ok {
		if id = cssIdent(id); id != "" {
			return "#" + id
		}
	}

	if class, ok := s.Attr("class"); ok {
		var picked []string
		for _, c := range strings.Fields(class) {
			if c = cssIdent(c); c != "" {
				picked = append(picked, c)
			}
			if len(picked) == 2 {
				break
			}
		}
		if len(picked) > 0 {
			return tag + "." + strings.Join(picked, ".")
		}
	}

	switch tag {
	case "article", "main":
		return tag
	}
	return ""
}

// cssIdent returns s when it is a simple CSS identifier (so it can be dropped into a
// selector without escaping), else "". Rejects empty, leading-digit, and tokens with
// characters outside [A-Za-z0-9_-] (e.g. Tailwind's "md:flex").
func cssIdent(s string) string {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return ""
	}
	for _, r := range s {
		switch {
		case r == '-', r == '_',
			r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		default:
			return ""
		}
	}
	return s
}
