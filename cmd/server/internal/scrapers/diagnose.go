package scrapers

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/trace"

	"github.com/PuerkitoBio/goquery"
	"github.com/mmcdole/gofeed"
)

// maxDiagnoseBody caps how many bytes a raw fetch reads, so a misbehaving
// endpoint can't exhaust memory during a diagnose.
const maxDiagnoseBody = 8 << 20 // 8 MiB

// RawResponse holds the raw bytes and metadata of a single feed HTTP fetch,
// captured before any parsing so failures stay inspectable.
type RawResponse struct {
	URL         string
	FinalURL    string // after redirects
	Status      int
	ContentType string
	Body        []byte
	Duration    time.Duration
}

// FetchRaw performs a GET for a feed URL through the shared anonymized HTTP
// client — the same client gofeed uses for ParseURL — and returns the raw body
// plus response metadata. Custom headers are overlaid after the anon profile, so
// per-feed headers behave exactly as they do on the normal fetch path.
func FetchRaw(feedURL string, headers map[string]string) (RawResponse, error) {
	raw := RawResponse{URL: feedURL, FinalURL: feedURL}

	ctx := contextWithHeaders(context.Background(), headers)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return raw, err
	}

	client := GetSharedAnonymizedScraper().HTTPClient()
	start := time.Now()
	resp, err := client.Do(req)
	raw.Duration = time.Since(start)
	if err != nil {
		return raw, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDiagnoseBody))
	if err != nil {
		return raw, err
	}

	raw.Status = resp.StatusCode
	raw.ContentType = resp.Header.Get("Content-Type")
	raw.Body = body
	if resp.Request != nil && resp.Request.URL != nil {
		raw.FinalURL = resp.Request.URL.String()
	}
	return raw, nil
}

// FeedInspection bundles a feed diagnosis with sample article links and the
// detected feed title, for scaffolding a feed configuration.
type FeedInspection struct {
	Diagnosis   models.FeedDiagnosis
	SampleLinks []string
	// SampleContentChars holds the plain-text length of each sampled entry's feed
	// content (content:encoded, falling back to description), aligned 1:1 with
	// SampleLinks. It lets a caller tell whether the feed already ships full bodies.
	SampleContentChars []int
	// SampleContent holds the plain text of each sampled entry's feed content, aligned
	// 1:1 with SampleLinks. A caller can match it against the real article page to
	// confirm the body is fully in the feed, not merely long (a teaser can be long too).
	SampleContent []string
	Title         string
}

// defaultSampleLinks is the number of article links InspectFeedURL returns when
// the caller does not specify a cap.
const defaultSampleLinks = 5

// InspectFeedURL fetches a feed URL, attempts to parse it, and returns a full
// diagnosis plus up to maxLinks sample article links and the feed title (when the
// feed parses). It is read-only: nothing is stored. The raw body is always saved
// to disk (even when tracing is off) so the offending bytes stay inspectable.
func InspectFeedURL(feedURL string, headers map[string]string, maxLinks int) FeedInspection {
	raw, err := FetchRaw(feedURL, headers)
	if err != nil {
		// Network-level failure: there is no body to analyze.
		return FeedInspection{Diagnosis: models.FeedDiagnosis{
			URL:             feedURL,
			FinalURL:        raw.FinalURL,
			FetchDurationMs: raw.Duration.Milliseconds(),
			ParseError:      err.Error(),
			Verdict:         fmt.Sprintf("fetch failed: %v", err),
		}}
	}

	if maxLinks <= 0 {
		maxLinks = defaultSampleLinks
	}

	var insp FeedInspection
	var itemCount int
	feed, parseErr := gofeed.NewParser().Parse(bytes.NewReader(raw.Body))
	if parseErr == nil && feed != nil {
		itemCount = len(feed.Items)
		insp.Title = feed.Title
		for _, it := range feed.Items {
			if it.Link == "" {
				continue
			}
			text := feedItemText(it)
			insp.SampleLinks = append(insp.SampleLinks, it.Link)
			insp.SampleContent = append(insp.SampleContent, text)
			insp.SampleContentChars = append(insp.SampleContentChars, len([]rune(text)))
			if len(insp.SampleLinks) >= maxLinks {
				break
			}
		}
	}

	diag := AnalyzeFeedBody(raw, parseErr, itemCount)
	diag.RawBodyPath = trace.SaveDiagnostic(hostOf(feedURL), raw.Status, raw.ContentType, raw.Body)

	// When the URL is itself an HTML page (not a feed), try to discover the real
	// feed so autoconfig can be pointed at it instead of the landing page.
	if diag.FeedTypeGuess == "html" {
		cands := discoverFeedLinks(raw.Body, raw.FinalURL)
		diag.DiscoveredFeeds = validateFeedCandidates(cands, headers, raw.FinalURL)
		// An HTML index page has no feed title, so fall back to its <title> so the
		// generated config can still carry a human-readable name.
		insp.Title = extractHTMLTitle(raw.Body)
	}

	insp.Diagnosis = diag
	return insp
}

// feedItemText returns the plain text of an item's feed content, preferring
// content:encoded over description (the same precedence the real fetch uses). HTML is
// stripped so tag soup doesn't inflate a short stub past the bar.
func feedItemText(item *gofeed.Item) string {
	html := item.Content
	if html == "" {
		html = item.Description
	}
	if html == "" {
		return ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return strings.TrimSpace(html)
	}
	return strings.TrimSpace(doc.Text())
}

// utf8BOM is the UTF-8 byte order mark.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// stripLeadingBOMs removes any run of UTF-8 BOMs at the start of body. Some pages
// emit the BOM more than once; the HTML/XML parsers only tolerate one, and the stray
// BOMs are non-whitespace character tokens that corrupt the parse tree.
func stripLeadingBOMs(body []byte) []byte {
	for bytes.HasPrefix(body, utf8BOM) {
		body = body[len(utf8BOM):]
	}
	return body
}

// extractHTMLTitle returns the trimmed text of an HTML page's <title> element, or
// "" when the body can't be parsed or has no title.
func extractHTMLTitle(body []byte) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(stripLeadingBOMs(body)))
	if err != nil {
		return ""
	}
	// Prefer the head title, but fall back to any title: a malformed page can foster
	// the title into the body, where "head title" wouldn't match.
	t := strings.TrimSpace(doc.Find("head title").First().Text())
	if t == "" {
		t = strings.TrimSpace(doc.Find("title").First().Text())
	}
	return t
}

// DiagnoseFeedURL fetches a feed URL and returns a structured diagnosis. It is a
// thin wrapper over InspectFeedURL for callers that only need the diagnosis.
func DiagnoseFeedURL(feedURL string, headers map[string]string) models.FeedDiagnosis {
	return InspectFeedURL(feedURL, headers, 0).Diagnosis
}

// AnalyzeFeedBody turns a raw response and its parse outcome into a structured
// diagnosis. It is a pure function over the bytes (no IO) so it can be unit
// tested directly against crafted payloads.
func AnalyzeFeedBody(raw RawResponse, parseErr error, itemCount int) models.FeedDiagnosis {
	diag := models.FeedDiagnosis{
		URL:             raw.URL,
		FinalURL:        raw.FinalURL,
		HTTPStatus:      raw.Status,
		ContentType:     raw.ContentType,
		ContentLength:   len(raw.Body),
		ItemCount:       itemCount,
		FetchDurationMs: raw.Duration.Milliseconds(),
		FeedTypeGuess:   guessFeedType(raw.Body),
		DeclaredCharset: declaredCharset(raw.Body, raw.ContentType),
		BodySnippet:     snippet(raw.Body, 200),
	}
	if parseErr != nil {
		diag.ParseError = parseErr.Error()
	}
	if off := firstInvalidUTF8(raw.Body); off >= 0 {
		diag.InvalidUTF8At = &off
		diag.HexDump = hexAround(raw.Body, off)
	}

	diag.Verdict = verdict(diag, raw.Body)
	return diag
}

var (
	xmlEncodingRe = regexp.MustCompile(`(?i)<\?xml[^>]*\bencoding\s*=\s*["']([^"']+)["']`)
	ctCharsetRe   = regexp.MustCompile(`(?i)charset\s*=\s*"?([^"\s;]+)`)
)

// guessFeedType sniffs the leading bytes to classify what came back, independent
// of the (often wrong or missing) Content-Type header.
func guessFeedType(body []byte) string {
	trimmed := stripLeadingBOMs(body)
	trimmed = bytes.TrimLeft(trimmed, " \t\r\n")
	if len(trimmed) == 0 {
		return "empty"
	}

	// JSON Feed: a JSON object that names the spec or carries an items array.
	if trimmed[0] == '{' {
		head := strings.ToLower(string(peek(trimmed, 2048)))
		if strings.Contains(head, "jsonfeed.org") || strings.Contains(head, "\"items\"") {
			return "json-feed"
		}
		return "unknown"
	}

	head := strings.ToLower(string(peek(trimmed, 2048)))
	switch {
	case strings.Contains(head, "<rss"):
		return "rss"
	case strings.Contains(head, "<feed"):
		return "atom"
	case strings.Contains(head, "<!doctype html"), strings.Contains(head, "<html"):
		return "html"
	default:
		return "unknown"
	}
}

// declaredCharset reports the charset the payload claims (XML prolog first, then
// the Content-Type header). Returns "" when none is declared.
func declaredCharset(body []byte, contentType string) string {
	if m := xmlEncodingRe.FindSubmatch(peek(body, 512)); m != nil {
		return strings.ToLower(string(m[1]))
	}
	if m := ctCharsetRe.FindStringSubmatch(contentType); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// firstInvalidUTF8 returns the byte offset of the first invalid UTF-8 sequence,
// or -1 when the body is valid UTF-8.
func firstInvalidUTF8(body []byte) int {
	for i := 0; i < len(body); {
		r, size := utf8.DecodeRune(body[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// hexAround returns a hex dump of the bytes surrounding off, to expose the exact
// offending bytes (e.g. an 0xe9 from a Latin-1 "é").
func hexAround(body []byte, off int) string {
	start := off - 8
	if start < 0 {
		start = 0
	}
	end := off + 24
	if end > len(body) {
		end = len(body)
	}
	return strings.TrimRight(hex.Dump(body[start:end]), "\n")
}

// snippet returns the first n bytes of the body as a single-line printable
// string, with control characters collapsed to spaces.
func snippet(body []byte, n int) string {
	b := peek(body, n)
	var sb strings.Builder
	for _, r := range string(b) {
		switch {
		case r == utf8.RuneError:
			sb.WriteByte('.')
		case unicode.IsControl(r):
			sb.WriteByte(' ')
		default:
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(strings.Join(strings.Fields(sb.String()), " "))
}

// antiBotMarkers maps a body substring to the vendor/challenge it signals.
var antiBotMarkers = []struct{ needle, label string }{
	{"just a moment", "Cloudflare challenge"},
	{"cf-browser-verification", "Cloudflare challenge"},
	{"checking your browser", "Cloudflare challenge"},
	{"attention required", "Cloudflare block"},
	{"access denied", "access-denied page"},
	{"captcha", "CAPTCHA page"},
	{"enable javascript", "JavaScript-gated page"},
	{"are you a robot", "bot-check page"},
}

// antiBotMarker reports a recognised challenge/block page label, or "".
func antiBotMarker(body []byte) string {
	head := strings.ToLower(string(peek(body, 4096)))
	for _, m := range antiBotMarkers {
		if strings.Contains(head, m.needle) {
			return m.label
		}
	}
	return ""
}

// verdict renders the single actionable sentence shown to the user, in priority
// order: network/HTTP failures, then empty bodies, then anti-bot HTML, then
// charset problems, then a generic parse failure, then success.
func verdict(d models.FeedDiagnosis, body []byte) string {
	switch {
	case d.FeedTypeGuess == "empty":
		if d.HTTPStatus >= 400 {
			return fmt.Sprintf("empty body with HTTP %d", d.HTTPStatus)
		}
		return "empty response body"
	case d.FeedTypeGuess == "html":
		if label := antiBotMarker(body); label != "" {
			return fmt.Sprintf("%s (HTTP %d) — got HTML, not a feed", label, d.HTTPStatus)
		}
		return fmt.Sprintf("HTML page (HTTP %d), not a feed — likely a login wall or error page", d.HTTPStatus)
	case d.InvalidUTF8At != nil:
		cs := d.DeclaredCharset
		if cs == "" {
			cs = "undeclared"
		}
		return fmt.Sprintf("invalid UTF-8 at byte %d (declared charset: %s) — feed needs transcoding to UTF-8", *d.InvalidUTF8At, cs)
	case d.HTTPStatus >= 400:
		return fmt.Sprintf("HTTP %d — %s body", d.HTTPStatus, d.FeedTypeGuess)
	case d.ParseError != "":
		return fmt.Sprintf("parse failed (%s body): %s", d.FeedTypeGuess, d.ParseError)
	default:
		return fmt.Sprintf("valid %s feed, %d items", d.FeedTypeGuess, d.ItemCount)
	}
}

// peek returns the first n bytes of b (or all of b when shorter).
func peek(b []byte, n int) []byte {
	if len(b) > n {
		return b[:n]
	}
	return b
}

// hostOf extracts the host from a URL for use in a filename, falling back to
// "unknown" when the URL can't be parsed.
func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		return u.Host
	}
	return "unknown"
}
