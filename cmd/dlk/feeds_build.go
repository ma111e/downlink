package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// usableChars mirrors the server's minUsableChars threshold (scrapers.Usable):
// extracted content shorter than this is treated as a stub/teaser, not an article.
const usableChars = 500

// scrapeModes is the escalation order the builder follows: cheapest first,
// full_browser last (resource-heavy, but the reliable fallback).
var scrapeModes = []string{"static", "dynamic", "full_browser"}

// parseHeaders turns repeated "Key: Value" / "Key=Value" flags into a map.
func parseHeaders(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(raw))
	for _, h := range raw {
		sep := strings.IndexAny(h, ":=")
		if sep <= 0 {
			return nil, fmt.Errorf("invalid header %q (expected \"Key: Value\")", h)
		}
		k := strings.TrimSpace(h[:sep])
		v := strings.TrimSpace(h[sep+1:])
		out[k] = v
	}
	return out, nil
}

// createFeedBuildCommands returns the feed-config builder toolkit: the primitives
// the feed-config-builder skill (or a person) composes to go from a bare RSS URL
// to a working, selector-complete feed configuration.
func createFeedBuildCommands() []*cobra.Command {
	return []*cobra.Command{
		newGenerateCmd(),
		newFetchArticleCmd(),
		newTestSelectorCmd(),
		newProbeModesCmd(),
		newProbeHeadersCmd(),
	}
}

// ── generate ──────────────────────────────────────────────────────────────────

func newGenerateCmd() *cobra.Command {
	var headerFlags []string
	cmd := &cobra.Command{
		Use:   "generate <rss-url>",
		Short: "Scaffold a feed config from an RSS/Atom URL",
		Long: `Fetch and inspect a feed URL, then print a starter feed configuration plus the
sample article links to use when finding the article selector.

This is the entry point for building a feed config. It does not register or write
anything — it prints YAML you refine (typically via the feed-config-builder skill)
and finally paste into your feeds.yml.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headers, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()
			resp, err := client.InspectFeed(args[0], headers, 5)
			if err != nil {
				return fmt.Errorf("inspect feed: %w", err)
			}

			feedType := "rss"
			if resp.Diagnosis.GetFeedTypeGuess() == "atom" {
				feedType = "atom"
			}
			cfg := models.FeedConfig{
				URL:     args[0],
				Title:   resp.DetectedTitle,
				Type:    feedType,
				Enabled: true,
				Headers: headers,
			}

			if jsonOutput {
				return printJSON(map[string]any{
					"config":       cfg,
					"diagnosis":    resp.Diagnosis,
					"sample_links": resp.SampleLinks,
				})
			}

			printDiagnosis(resp.DetectedTitle, resp.Diagnosis, false)
			fmt.Println(styleSection.Render("── starter config ──"))
			out, _ := yaml.Marshal(models.FeedsFile{Feeds: []models.FeedConfig{cfg}})
			fmt.Println(string(out))
			printList("Sample article links (inspect these to find the selector)", resp.SampleLinks)
			fmt.Printf("\n%s find the article selector, then test it:\n", styleKey.Render("Next:"))
			fmt.Printf("  dlk feeds fetch-article <link> --mode static\n")
			fmt.Printf("  dlk feeds test-selector --url <link> --article \"<css>\" --mode static\n")
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Custom HTTP header \"Key: Value\" (repeatable)")
	return cmd
}

// ── fetch-article ─────────────────────────────────────────────────────────────

func newFetchArticleCmd() *cobra.Command {
	var headerFlags []string
	var mode string
	var full bool
	cmd := &cobra.Command{
		Use:   "fetch-article <article-url>",
		Short: "Fetch an article's page HTML in a scraping mode",
		Long: `Scrape a single article URL and print its page HTML, so you can find the CSS
selector that wraps the article body. Use --mode to pick how it is fetched:
static (default), dynamic (headless JS), or full_browser (heaviest).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headers, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			limit := 0
			if full {
				limit = 1 << 30 // effectively unlimited
			}
			client := getNewDownlinkClient()
			resp, err := client.InspectArticle(args[0], mode, headers, nil, limit)
			if err != nil {
				return fmt.Errorf("inspect article: %w", err)
			}
			if jsonOutput {
				return printJSON(resp)
			}
			if resp.Error != "" {
				fmt.Printf("%s %s (mode %s): %s\n", styleErr.Render("✗"), args[0], resp.GetModeUsed(), resp.Error)
				return nil
			}
			fmt.Printf("%s %s  mode=%s  html=%d bytes  %dms\n",
				styleOK.Render("✓"), args[0], resp.GetModeUsed(), resp.GetRawHtmlLen(), resp.GetDurationMs())
			fmt.Println(styleSection.Render("── page HTML ──"))
			fmt.Println(resp.GetHtml())
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Custom HTTP header \"Key: Value\" (repeatable)")
	cmd.Flags().StringVarP(&mode, "mode", "m", "static", "Scraping mode: static | dynamic | full_browser")
	cmd.Flags().BoolVar(&full, "full", false, "Print the entire page HTML (default caps it)")
	return cmd
}

// ── test-selector ─────────────────────────────────────────────────────────────

func newTestSelectorCmd() *cobra.Command {
	var headerFlags, urls []string
	var mode, article, cutoff, blacklist string
	cmd := &cobra.Command{
		Use:   "test-selector --url <article-url> --article <css>",
		Short: "Test selectors against one or more articles and score them",
		Long: `Extract content from one or more article URLs using the given selectors and show
the result, so you can judge whether a candidate selector is right. Pass --url
multiple times to score the selector's stability across several articles.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(urls) == 0 {
				return fmt.Errorf("at least one --url is required")
			}
			if article == "" {
				return fmt.Errorf("--article selector is required")
			}
			headers, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			sel := &protos.Selectors{Article: article, Cutoff: cutoff, Blacklist: blacklist}
			client := getNewDownlinkClient()

			type sample struct {
				URL       string `json:"url"`
				Matched   bool   `json:"matched"`
				Chars     int    `json:"chars"`
				Error     string `json:"error,omitempty"`
				Extracted string `json:"extracted,omitempty"`
			}
			var samples []sample
			var usable, totalChars int
			for _, u := range urls {
				resp, err := client.InspectArticle(u, mode, headers, sel, 0)
				if err != nil {
					samples = append(samples, sample{URL: u, Error: err.Error()})
					continue
				}
				s := sample{
					URL: u, Matched: resp.GetSelectorMatched(),
					Chars: int(resp.GetExtractedLen()), Error: resp.GetError(),
					Extracted: resp.GetExtracted(),
				}
				samples = append(samples, s)
				if s.Matched && s.Chars >= usableChars {
					usable++
					totalChars += s.Chars
				}
			}

			score := float64(usable) / float64(len(urls))
			if jsonOutput {
				return printJSON(map[string]any{
					"selectors": map[string]string{"article": article, "cutoff": cutoff, "blacklist": blacklist},
					"mode":      mode,
					"usable":    usable,
					"samples":   len(urls),
					"score":     score,
					"results":   samples,
				})
			}

			fmt.Printf("\n%s  article=%q  mode=%s\n", styleBold.Render("Selector test:"), article, mode)
			for _, s := range samples {
				switch {
				case s.Error != "":
					fmt.Printf("  %s %-50s %s\n", styleErr.Render("✗"), s.URL, styleErr.Render(s.Error))
				case s.Matched && s.Chars >= usableChars:
					fmt.Printf("  %s %-50s %d chars\n", styleOK.Render("✓"), s.URL, s.Chars)
				default:
					note := "no match"
					if s.Matched {
						note = fmt.Sprintf("only %d chars", s.Chars)
					}
					fmt.Printf("  %s %-50s %s\n", styleWarn.Render("⚠"), s.URL, styleWarn.Render(note))
				}
			}
			verdict := styleErr
			if score >= 0.8 {
				verdict = styleOK
			} else if score >= 0.5 {
				verdict = styleWarn
			}
			fmt.Printf("  %s %s  (%d/%d usable)\n", styleKey.Render("Score"), verdict.Render(fmt.Sprintf("%.2f", score)), usable, len(urls))

			// Show one extracted sample so the human can eyeball quality.
			for _, s := range samples {
				if s.Matched && s.Extracted != "" {
					fmt.Printf("\n%s (%s)\n", styleSection.Render("── extracted sample ──"), s.URL)
					fmt.Println(truncate(s.Extracted, 1200))
					break
				}
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&urls, "url", nil, "Article URL to test (repeatable)")
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Custom HTTP header \"Key: Value\" (repeatable)")
	cmd.Flags().StringVarP(&mode, "mode", "m", "static", "Scraping mode: static | dynamic | full_browser")
	cmd.Flags().StringVar(&article, "article", "", "Article content CSS selector (required)")
	cmd.Flags().StringVar(&cutoff, "cutoff", "", "Cutoff CSS selector (content after it is dropped)")
	cmd.Flags().StringVar(&blacklist, "blacklist", "", "Blacklist CSS selector (elements removed)")
	return cmd
}

// ── probe-modes ───────────────────────────────────────────────────────────────

func newProbeModesCmd() *cobra.Command {
	var headerFlags []string
	var article, cutoff, blacklist string
	cmd := &cobra.Command{
		Use:   "probe-modes <article-url>",
		Short: "Find the cheapest scraping mode that yields full content",
		Long: `Try each scraping mode in priority order (static -> dynamic -> full_browser)
against an article and report what each yields, so you pick the cheapest one that
works. full_browser is resource-heavy and used only when nothing lighter succeeds.

Pass --article to judge by the content that selector extracts; otherwise modes are
compared by rendered HTML size.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headers, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			var sel *protos.Selectors
			if article != "" {
				sel = &protos.Selectors{Article: article, Cutoff: cutoff, Blacklist: blacklist}
			}
			client := getNewDownlinkClient()

			type modeResult struct {
				Mode       string `json:"mode"`
				Chars      int    `json:"extracted_chars"`
				HTMLLen    int    `json:"raw_html_len"`
				Matched    bool   `json:"selector_matched"`
				Usable     bool   `json:"usable"`
				DurationMs int64  `json:"duration_ms"`
				Error      string `json:"error,omitempty"`
			}
			var results []modeResult
			recommended := ""
			for _, m := range scrapeModes {
				resp, err := client.InspectArticle(args[0], m, headers, sel, 0)
				r := modeResult{Mode: m}
				if err != nil {
					r.Error = err.Error()
					results = append(results, r)
					continue
				}
				r.Chars = int(resp.GetExtractedLen())
				r.HTMLLen = int(resp.GetRawHtmlLen())
				r.Matched = resp.GetSelectorMatched()
				r.DurationMs = resp.GetDurationMs()
				r.Error = resp.GetError()
				if sel != nil {
					r.Usable = r.Matched && r.Chars >= usableChars
				} else {
					r.Usable = r.Error == "" && r.HTMLLen > 0
				}
				results = append(results, r)
				if recommended == "" && r.Usable {
					recommended = m // first usable = cheapest, since scrapeModes is ordered
				}
			}

			if jsonOutput {
				return printJSON(map[string]any{"url": args[0], "results": results, "recommended": recommended})
			}

			fmt.Printf("\n%s %s\n", styleBold.Render("Mode probe:"), args[0])
			for _, r := range results {
				mark := styleWarn.Render("⚠")
				if r.Error != "" {
					mark = styleErr.Render("✗")
				} else if r.Usable {
					mark = styleOK.Render("✓")
				}
				detail := fmt.Sprintf("html=%d", r.HTMLLen)
				if sel != nil {
					detail = fmt.Sprintf("extracted=%d matched=%v", r.Chars, r.Matched)
				}
				heavy := ""
				if r.Mode == "full_browser" {
					heavy = styleDim.Render("  (heavy)")
				}
				line := fmt.Sprintf("%-13s %s  %dms%s", r.Mode, detail, r.DurationMs, heavy)
				if r.Error != "" {
					line = fmt.Sprintf("%-13s %s", r.Mode, styleErr.Render(r.Error))
				}
				fmt.Printf("  %s %s\n", mark, line)
			}
			if recommended != "" {
				fmt.Printf("  %s %s\n", styleKey.Render("Recommend"), styleOK.Render(recommended))
			} else {
				fmt.Printf("  %s %s\n", styleKey.Render("Recommend"), styleErr.Render("no mode produced usable content"))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Custom HTTP header \"Key: Value\" (repeatable)")
	cmd.Flags().StringVar(&article, "article", "", "Judge each mode by what this selector extracts")
	cmd.Flags().StringVar(&cutoff, "cutoff", "", "Cutoff CSS selector")
	cmd.Flags().StringVar(&blacklist, "blacklist", "", "Blacklist CSS selector")
	return cmd
}

// ── probe-headers ─────────────────────────────────────────────────────────────

func newProbeHeadersCmd() *cobra.Command {
	var headerFlags []string
	cmd := &cobra.Command{
		Use:   "probe-headers <feed-url>",
		Short: "Find header combinations that unblock a feed",
		Long: `When a feed 403s or returns an anti-bot/HTML page, try a few common header
combinations (Referer, desktop User-Agent, RSS Accept) and report which ones
return a valid feed, so you can add the minimal working set to the config.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			extra, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			origin := originOf(args[0])
			desktopUA := "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0"

			combos := []struct {
				name    string
				headers map[string]string
			}{
				{"default", nil},
				{"+Referer", map[string]string{"Referer": origin}},
				{"+Referer +UA", map[string]string{"Referer": origin, "User-Agent": desktopUA}},
				{"+RSS Accept", map[string]string{"Accept": "application/rss+xml, application/atom+xml, application/xml;q=0.9, */*;q=0.8"}},
			}

			client := getNewDownlinkClient()
			type comboResult struct {
				Name    string            `json:"name"`
				Headers map[string]string `json:"headers"`
				Status  int               `json:"http_status"`
				Verdict string            `json:"verdict"`
				Usable  bool              `json:"usable"`
			}
			var results []comboResult
			var recommended map[string]string
			for _, c := range combos {
				h := mergeHeaders(c.headers, extra)
				resp, err := client.InspectFeed(args[0], h, 1)
				r := comboResult{Name: c.name, Headers: h}
				if err != nil {
					r.Verdict = err.Error()
					results = append(results, r)
					continue
				}
				d := resp.GetDiagnosis()
				r.Status = int(d.GetHttpStatus())
				r.Verdict = d.GetVerdict()
				guess := d.GetFeedTypeGuess()
				r.Usable = d.GetParseError() == "" && (guess == "rss" || guess == "atom" || guess == "json-feed")
				results = append(results, r)
				if recommended == nil && r.Usable {
					recommended = h
				}
			}

			if jsonOutput {
				return printJSON(map[string]any{"url": args[0], "results": results, "recommended": recommended})
			}

			fmt.Printf("\n%s %s\n", styleBold.Render("Header probe:"), args[0])
			for _, r := range results {
				mark := styleErr.Render("✗")
				if r.Usable {
					mark = styleOK.Render("✓")
				}
				fmt.Printf("  %s %-14s HTTP %d  %s\n", mark, r.Name, r.Status, styleDim.Render(r.Verdict))
			}
			if recommended != nil {
				fmt.Printf("  %s\n", styleKey.Render("Recommended headers:"))
				for _, k := range sortedKeys(recommended) {
					fmt.Printf("    %s: %s\n", k, recommended[k])
				}
			} else {
				fmt.Printf("  %s %s\n", styleKey.Render("Recommend"), styleErr.Render("no tested combination unblocked the feed"))
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Extra header applied to every combination (repeatable)")
	return cmd
}

// ── small shared helpers ──────────────────────────────────────────────────────

func printJSON(v any) error {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

func printList(label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n%s\n", styleSection.Render(label))
	for _, it := range items {
		fmt.Printf("  - %s\n", it)
	}
}

// originOf returns scheme://host/ for a URL, for use as a Referer.
func originOf(rawURL string) string {
	if i := strings.Index(rawURL, "://"); i > 0 {
		rest := rawURL[i+3:]
		host := rest
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			host = rest[:j]
		}
		return rawURL[:i+3] + host + "/"
	}
	return rawURL
}

func mergeHeaders(a, b map[string]string) map[string]string {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
