package scrapers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/playwright-community/playwright-go"
	log "github.com/sirupsen/logrus"
	"github.com/tebeka/selenium"

	"github.com/gocolly/colly/v2"
)

// AnonymizedScraper wraps a Colly collector and implements anonymization on each request.
type AnonymizedScraper struct {
	Collector  *colly.Collector
	userAgents []string
	// Playwright related fields
	pw             *playwright.Playwright
	browser        playwright.Browser
	browserContext playwright.BrowserContext
	playwrightInit bool
}

// NewAnonymizedScraper creates a new instance of AnonymizedScraper for the given domain.
func NewAnonymizedScraper(domain string) *AnonymizedScraper {
	// Create a new collector with the allowed domain
	c := colly.NewCollector(
		colly.AllowedDomains(domain),
	)

	// Setup a custom HTTP client with advanced configurations
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // Bypass SSL verification
		},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
	c.SetClient(client)

	scraper := &AnonymizedScraper{
		Collector: c,
		userAgents: []string{
			"Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.0.0 Safari/537.3",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1.1 Safari/605.1.1",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/134.0.0.0 Safari/537.36 Edg/134.0.3124.85",
		},
		playwrightInit: false,
	}

	// Attach the anonymization logic to every request
	c.OnRequest(func(r *colly.Request) {
		scraper.anonymizeRequest(r)
	})

	return scraper
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

// ScrapeContent visits the URL, processes the HTML content and returns the largest content block.
func (s *AnonymizedScraper) ScrapeContent(url string) (dom *goquery.Selection, err error) {
	// Process the HTML content once the body is received
	s.Collector.OnHTML("body", func(e *colly.HTMLElement) {
		dom = e.DOM
	})

	// Dump the request
	// s.Collector.OnRequest(func(r *colly.Request) {
	// 	spew.Dump(r)
	// })

	// Visit the URL
	if err := s.Collector.Visit(url); err != nil {
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

	// Initialize Playwright (no browser install needed — using Lightpanda via Docker)
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
func (s *AnonymizedScraper) ScrapeContentWithPlaywright(url string) (*goquery.Selection, error) {
	// Initialize Playwright if not already done
	if err := s.initPlaywright(); err != nil {
		return nil, err
	}

	// Lightpanda closes its target after every page lifecycle, so always reconnect
	// before each scrape. This is a cheap WebSocket reconnect — the pw process stays alive.
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
