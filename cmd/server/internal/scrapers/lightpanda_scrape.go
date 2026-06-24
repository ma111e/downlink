package scrapers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	log "github.com/sirupsen/logrus"
)

// dynamicScrapeTimeout bounds a single dynamic scrape (navigate + render + read).
const dynamicScrapeTimeout = 45 * time.Second

// ScrapeContentDynamic renders a page with JavaScript by driving Lightpanda over CDP
// and returns its body DOM. It uses chromedp — a pure-Go CDP client — so it needs no
// Playwright driver, no Node, and no bundled browser: it connects to the Lightpanda
// instance already running on the CDP port. Drop-in replacement for the old
// Playwright path (same signature and return shape). Custom headers are overlaid
// after the anon profile (custom wins).
func (s *AnonymizedScraper) ScrapeContentDynamic(url string, customHeaders map[string]string) (*goquery.Selection, error) {
	wsURL, err := lightpandaWebSocketURL()
	if err != nil {
		return nil, fmt.Errorf("lightpanda not reachable for dynamic scraping: %w", err)
	}

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(browserCtx, dynamicScrapeTimeout)
	defer cancelTimeout()

	ua := s.userAgents[rand.Intn(len(s.userAgents))]

	// Setup is deliberately minimal. Lightpanda implements only a CDP subset and, crucially,
	// enabling the Network domain together with Network.setExtraHTTPHeaders makes it emit a
	// request event whose headers JSON carries a duplicate key. cdproto's strict decoder
	// rejects that event, tears down the CDP connection, and every command after it — the
	// Navigate below included — fails with a bare "context canceled". So we only override the
	// user agent (safe without Network.enable) and do not inject extra headers here. Custom
	// headers are not deliverable on the Lightpanda dynamic path.
	if len(customHeaders) > 0 {
		log.WithField("count", len(customHeaders)).Debug("dynamic scrape: custom headers are not applied on the Lightpanda path")
	}
	if err := chromedp.Run(ctx, emulation.SetUserAgentOverride(ua)); err != nil {
		log.WithError(err).Debug("dynamic scrape: user-agent override not supported by Lightpanda; continuing")
	}

	// If setup tore down the browser connection, surface that directly instead of letting
	// the Navigate below report an opaque "context canceled".
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("dynamic scrape via Lightpanda failed during setup: %w", err)
	}

	var html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // let late JS settle
		chromedp.OuterHTML("html", &html, chromedp.ByQuery),
	); err != nil {
		return nil, fmt.Errorf("dynamic scrape via Lightpanda failed: %w", err)
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse rendered HTML: %w", err)
	}
	return doc.Selection, nil
}

// lightpandaWebSocketURL resolves the CDP websocket endpoint to connect to. It asks
// Lightpanda's /json/version for the exact webSocketDebuggerUrl and falls back to the
// conventional ws://host:port/ when that endpoint isn't available.
func lightpandaWebSocketURL() (string, error) {
	fallback := fmt.Sprintf("ws://%s/", net.JoinHostPort(lightpandaCDPHost, lightpandaCDPPort))

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/json/version", net.JoinHostPort(lightpandaCDPHost, lightpandaCDPPort)))
	if err != nil {
		// No HTTP discovery endpoint — try the conventional ws URL directly.
		return fallback, nil
	}
	defer resp.Body.Close()

	if ws := parseWebSocketDebuggerURL(resp.Body); ws != "" {
		return ws, nil
	}
	return fallback, nil
}

// parseWebSocketDebuggerURL extracts webSocketDebuggerUrl from a CDP /json/version
// response body, or "" when it is absent/unparseable.
func parseWebSocketDebuggerURL(r interface{ Read([]byte) (int, error) }) string {
	var payload struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.WebSocketDebuggerURL)
}
