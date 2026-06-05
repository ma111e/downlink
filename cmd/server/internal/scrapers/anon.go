package scrapers

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/trace"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
	log "github.com/sirupsen/logrus"
	"github.com/tebeka/selenium"

	"github.com/gocolly/colly/v2"
)

// AnonymizedScraper owns the shared, expensive-to-build scraping state (the reused
// Playwright browser/context and the User-Agent pool) and applies anonymization to
// each request. Colly collectors are NOT held here: ScrapeContent builds a fresh,
// request-scoped collector per call so concurrent scrapes never share mutable state.
type AnonymizedScraper struct {
	userAgents []string
	// Playwright related fields
	pw             *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	playwrightInit bool
}

// newAnonTransport builds the HTTP transport used by both the colly collector and
// the anon HTTP client: relaxed TLS verification and bounded dial timeouts.
func newAnonTransport() *http.Transport {
	return &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Bypass SSL verification
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
}

// NewAnonymizedScraper creates a new AnonymizedScraper.
func NewAnonymizedScraper() *AnonymizedScraper {
	return &AnonymizedScraper{
		userAgents: []string{
			"Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.3",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1.1 Safari/605.1.1",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36 Edg/134.0.3124.85",
		},
		playwrightInit: false,
	}
}

// newCollector builds a fresh colly collector for a single ScrapeContent call. Each
// call gets its own collector — and thus its own visited-set, handler list, and HTTP
// client — so repeated and concurrent scrapes can neither dedup-skip each other nor
// race on shared callbacks. No AllowedDomains is set: like the HTTPClient path, a
// one-shot caller-driven fetch should follow redirects across hosts.
func (s *AnonymizedScraper) newCollector() *colly.Collector {
	c := colly.NewCollector()
	c.SetClient(&http.Client{
		Transport: newAnonTransport(),
		Timeout:   60 * time.Second,
	})

	// Apply the anon profile to every request.
	c.OnRequest(func(r *colly.Request) {
		s.anonymizeRequest(r)
	})

	// Overlay per-request custom headers (carried in colly.Context) after the
	// anonymization step so caller-supplied headers win over the anon profile.
	c.OnRequest(func(r *colly.Request) {
		if custom, ok := r.Ctx.GetAny(customHeadersKey).(map[string]string); ok {
			for k, v := range custom {
				r.Headers.Set(k, v)
			}
		}
	})

	return c
}

// HTTPClient returns an *http.Client that carries this scraper's anon profile
// (rotating User-Agent from s.userAgents + spoofed headers + Alt-Used) on every
// request via anonRoundTripper. Unlike the colly path it follows redirects across
// hosts and imposes no domain allowlist, so it suits feed/RSS fetching.
func (s *AnonymizedScraper) HTTPClient() *http.Client {
	return &http.Client{
		Transport: &anonRoundTripper{
			base:       newAnonTransport(),
			userAgents: s.userAgents,
		},
		Timeout: 60 * time.Second,
	}
}

// headersCtxKey carries caller-supplied custom HTTP headers through a request's
// context so anonRoundTripper can overlay them after the anon profile.
type headersCtxKey struct{}

// contextWithHeaders attaches custom HTTP headers to ctx. anonRoundTripper applies
// them last, so they take precedence over the anon profile (custom headers win).
func contextWithHeaders(ctx context.Context, headers map[string]string) context.Context {
	if len(headers) == 0 {
		return ctx
	}
	return context.WithValue(ctx, headersCtxKey{}, headers)
}

// customHeadersKey is the colly.Context key under which per-request custom headers
// are carried so they can be overlaid after anonymizeRequest (custom headers win).
const customHeadersKey = "customHeaders"

// anonRoundTripper applies the anon profile to every outbound request before
// delegating to a base RoundTripper. Because it runs at the transport layer it
// also applies on each redirect hop. It rotates the User-Agent from the same pool
// the owning AnonymizedScraper uses.
type anonRoundTripper struct {
	base       http.RoundTripper
	userAgents []string
}

func (t *anonRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// RoundTrippers must not mutate the supplied request; clone before editing.
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", t.userAgents[rand.Intn(len(t.userAgents))])

	// Spoof browser headers. Accept-Encoding is intentionally omitted so Go's
	// transport negotiates gzip and decompresses transparently (it has no brotli
	// support, so advertising "br" would risk a body we cannot decode).
	r.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	r.Header.Set("Accept-Language", "en-US,en;q=0.5")
	r.Header.Set("DNT", "1")
	r.Header.Set("Connection", "keep-alive")
	r.Header.Set("Upgrade-Insecure-Requests", "1")
	if r.URL != nil {
		r.Header.Set("Alt-Used", r.URL.Host)
	}

	// Overlay caller-supplied custom headers last so they win over the profile.
	// Accept-Encoding is skipped: Go's transport must own it (no brotli support).
	if custom, ok := r.Context().Value(headersCtxKey{}).(map[string]string); ok {
		for k, v := range custom {
			if http.CanonicalHeaderKey(k) == "Accept-Encoding" {
				continue
			}
			r.Header.Set(k, v)
		}
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(r)

	// When tracing is on, tee the raw (already-decompressed) response body to
	// disk so non-UTF-8 / malformed feed payloads can be inspected verbatim.
	// Guarded by Enabled() so the normal path never buffers the body.
	if err == nil && resp != nil && trace.Enabled() && req.Method == http.MethodGet {
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr == nil {
			trace.HTTP(req.Method, req.URL.String(), resp.StatusCode, resp.Header.Get("Content-Type"), body, time.Since(start))
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}

	return resp, err
}

// anonymizeRequest applies header spoofing, rotates the User-Agent, and adds a random delay.
func (s *AnonymizedScraper) anonymizeRequest(r *colly.Request) {
	// Rotate user agent
	r.Headers.Set("User-Agent", s.userAgents[rand.Intn(len(s.userAgents))])

	// Spoof additional headers
	headers := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.5",
		"Accept-Encoding":           "gzip, deflate, br",
		"DNT":                       "1",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
	}
	for k, v := range headers {
		r.Headers.Set(k, v)
	}

	// Randomize request timing to mimic human browsing behavior
	time.Sleep(time.Duration(rand.Intn(3000)) * time.Millisecond)
}

// ScrapeContent visits the URL, processes the HTML content and returns the largest
// content block. Custom headers, when provided, are carried via colly.Context and
// overlaid after the anon profile so they take precedence (custom headers win).
func (s *AnonymizedScraper) ScrapeContent(url string, headers map[string]string) (dom *goquery.Selection, err error) {
	// A fresh collector per call: the OnHTML handler and resulting dom are scoped to
	// this invocation, so concurrent scrapes never overwrite each other's result.
	c := s.newCollector()
	c.OnHTML("body", func(e *colly.HTMLElement) {
		dom = e.DOM
	})

	// Carry custom headers out-of-band so the overlay handler can apply them after
	// anonymizeRequest.
	ctx := colly.NewContext()
	if len(headers) > 0 {
		ctx.Put(customHeadersKey, headers)
	}

	if err := c.Request(http.MethodGet, url, nil, ctx, nil); err != nil {
		log.Printf("Failed to visit %s: %v", url, err)
		return nil, err
	}

	return dom, nil
}

// initPlaywright initializes the Playwright connection to Lightpanda running in Docker.
func (s *AnonymizedScraper) initPlaywright() error {
	if s.playwrightInit {
		return nil
	}

	// Initialize Playwright (no browser install needed, using Lightpanda via Docker)
	pw, err := playwright.Run()
	if err != nil {
		log.Errorf("Failed to start playwright: %v", err)
		return fmt.Errorf("failed to start playwright: %w", err)
	}
	s.pw = pw

	// Connect to Lightpanda running in Docker on port 9222
	cdpURL := "http://localhost:9222"

	browser, err := pw.Chromium.ConnectOverCDP(cdpURL)
	if err != nil {
		s.pw.Stop()
		log.Errorf("Could not connect to Lightpanda CDP at %s: %v", cdpURL, err)
		return fmt.Errorf("could not connect to Lightpanda CDP at %s: %w", cdpURL, err)
	}
	s.browser = browser

	// Use the default browser context provided by Lightpanda
	contexts := browser.Contexts()
	if len(contexts) > 0 {
		s.browserContext = contexts[0]
	} else {
		// Fallback: create a new context if none exists
		browserContext, err := browser.NewContext(playwright.BrowserNewContextOptions{
			UserAgent: playwright.String(s.userAgents[rand.Intn(len(s.userAgents))]),
		})
		if err != nil {
			log.Errorf("Failed to create browser context: %v", err)
			s.browser.Close()
			s.pw.Stop()
			return fmt.Errorf("failed to create browser context: %w", err)
		}
		s.browserContext = browserContext
	}
	s.playwrightInit = true

	return nil
}

// ClosePlaywright cleans up Playwright resources.
func (s *AnonymizedScraper) ClosePlaywright() {
	if !s.playwrightInit {
		return
	}

	if s.browserContext != nil {
		s.browserContext.Close()
	}

	if s.browser != nil {
		s.browser.Close()
	}

	if s.pw != nil {
		s.pw.Stop()
	}

	s.playwrightInit = false
}

// func (s *AnonymizedScraper) ScrapeContentWithPlaywright(url string) (*goquery.Selection, error) {
//     if err := s.initPlaywright(); err != nil {
//         return nil, err
//     }

//     // Create a fresh context per request instead of reusing the shared one
//     ctx, err := s.browser.NewContext(playwright.BrowserNewContextOptions{
//         UserAgent: playwright.String(s.userAgents[rand.Intn(len(s.userAgents))]),
//     })
//     if err != nil {
//         return nil, fmt.Errorf("failed to create browser context: %w", err)
//     }
//     defer ctx.Close() // scoped to this call only

//     page, err := ctx.NewPage()

// reconnectBrowser reconnects to Lightpanda CDP, refreshing the browser and context
// without restarting the Playwright instance.
func (s *AnonymizedScraper) reconnectBrowser() error {
	if s.browser != nil {
		s.browser.Close()
	}

	browser, err := s.pw.Chromium.ConnectOverCDP("http://localhost:9222")
	if err != nil {
		return fmt.Errorf("could not reconnect to Lightpanda: %w", err)
	}
	s.browser = browser

	contexts := browser.Contexts()
	if len(contexts) > 0 {
		s.browserContext = contexts[0]
	} else {
		ctx, err := browser.NewContext(playwright.BrowserNewContextOptions{
			UserAgent: playwright.String(s.userAgents[rand.Intn(len(s.userAgents))]),
		})
		if err != nil {
			s.browser.Close()
			return fmt.Errorf("failed to create browser context after reconnect: %w", err)
		}
		s.browserContext = ctx
	}

	return nil
}

// ScrapeContentWithPlaywright uses Playwright to fetch a page,
// waiting for JavaScript to fully load before retrieving the DOM.
// Custom headers, when provided, are overlaid after the anon profile (custom wins).
func (s *AnonymizedScraper) ScrapeContentWithPlaywright(url string, customHeaders map[string]string) (*goquery.Selection, error) {
	// Initialize Playwright if not already done
	if err := s.initPlaywright(); err != nil {
		return nil, err
	}

	// Lightpanda closes its target after every page lifecycle, so always reconnect
	// before each scrape. This is a cheap WebSocket reconnect; the pw process stays alive.
	if err := s.reconnectBrowser(); err != nil {
		return nil, err
	}

	page, err := s.browserContext.NewPage()
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}
	defer page.Close()

	log.Println("Navigating to URL with Playwright")

	// Set extra HTTP headers for anonymization
	headers := map[string]string{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
		"Accept-Language":           "en-US,en;q=0.5",
		"Accept-Encoding":           "gzip, deflate, br",
		"DNT":                       "1",
		"Connection":                "keep-alive",
		"Upgrade-Insecure-Requests": "1",
	}
	// Overlay caller-supplied custom headers last so they win over the profile.
	for k, v := range customHeaders {
		headers[k] = v
	}
	err = page.SetExtraHTTPHeaders(headers)
	if err != nil {
		log.Errorf("Failed to set extra HTTP headers: %v", err)
		return nil, fmt.Errorf("failed to set extra HTTP headers: %w", err)
	}

	// Navigate to the page with a timeout
	// Using a timeout option directly in the calls instead of a context since the Go Playwright package
	// doesn't use context in its API

	// Go to the URL
	if _, err = page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	}); err != nil {
		log.Errorf("Failed to navigate to %s: %v", url, err)
		return nil, fmt.Errorf("failed to navigate to URL: %w", err)
	}

	log.Println("Waiting for network to be idle")

	// Wait for network activity to be idle
	if err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: playwright.Float(10000), // 10 seconds
	}); err != nil {
		log.Warnf("Timeout waiting for network idle: %v", err)
		// Continue anyway, the page might be partially loaded
	}

	// Additional wait to ensure JavaScript execution completes
	time.Sleep(1 * time.Second)

	log.Println("Getting page content")

	// Get the page content
	content, err := page.Content()
	if err != nil {
		log.Errorf("Failed to get page content: %v", err)
		return nil, fmt.Errorf("failed to get page content: %w", err)
	}

	log.Println("Parsing HTML")

	// Parse the HTML with goquery
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		log.Errorf("Failed to parse HTML: %v", err)
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	return doc.Selection, nil
}

// waitForPageLoad waits for the page to be fully loaded including JavaScript execution
func waitForPageLoad(ctx context.Context, wd selenium.WebDriver) error {
	// Create ticker for polling
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return errors.New("timeout waiting for page to load")
		case <-ticker.C:
			// Check if document.readyState is complete
			readyState, err := wd.ExecuteScript("return document.readyState", nil)
			if err != nil {
				log.Warnf("Failed to execute script: %v", err)
				continue
			}

			// Check jQuery if it exists
			jQueryReady, err := wd.ExecuteScript("return typeof jQuery === 'undefined' || jQuery.active === 0", nil)
			if err != nil {
				log.Warnf("Failed to check jQuery status: %v", err)
				// Continue anyway, jQuery might not be used on the page
			}

			// Check for AJAX requests if fetch or XMLHttpRequest are being used
			fetchReady, err := wd.ExecuteScript(
				`return !window._seleniumFetchActive || window._seleniumFetchActive === 0`,
				nil)
			if err != nil {
				log.Warnf("Failed to check fetch status: %v", err)
				// Continue anyway, custom tracking might not be set up
			}

			// If all conditions are met, page is considered loaded
			if readyState == "complete" &&
				(jQueryReady == nil || jQueryReady.(bool)) &&
				(fetchReady == nil || fetchReady.(bool)) {
				// Additional delay to ensure all animations/transitions complete
				time.Sleep(1 * time.Second)
				return nil
			}
		}
	}
}
