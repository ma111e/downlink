package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"

	"charm.land/huh/v2"
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
		newInspectCmd(),
		newFetchArticleCmd(),
		newTestSelectorCmd(),
		newProbeModesCmd(),
		newProbeHeadersCmd(),
		newAutoConfigCmd(),
	}
}

// ── autoconfig ────────────────────────────────────────────────────────────────

func newAutoConfigCmd() *cobra.Command {
	var headerFlags []string
	var provider, model string
	var maxSteps int
	var yes, verbose bool
	var updateFile string
	cmd := &cobra.Command{
		Use:   "autoconfig <rss-url>",
		Short: "Let an LLM discover a feed's config on its own",
		Long: `Run downlink's autonomous agent: it probes and locks the scraping mode and
headers, then the configured LLM ranks and tests article selectors in that locked mode
and prints a finished feed config — no interactive session required.

If the URL is a web page rather than a feed, autoconfig first discovers the page's
RSS/Atom feeds and lets you pick which one to configure.

The agent streams its steps as it works. The final YAML is printed for you to paste
into your feeds.yml. Pass --update <file> to merge the result into an existing feeds
file instead (showing a diff when the feed is already present).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			headers, err := parseHeaders(headerFlags)
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()

			// Pre-flight: if the URL is a landing page rather than a feed, discover
			// the real feed and (interactively) pick which one to configure.
			targetURL, err := resolveFeedURL(client, args[0], headers, yes)
			if err != nil {
				return err
			}

			// Resolve server-side so the confirmation shows the model the run uses.
			// Same resolution AutoConfigFeed does, so failing here fails fast.
			providerType, modelName, err := client.ResolveLLM(provider, model)
			if err != nil {
				return fmt.Errorf("resolve model: %w", err)
			}

			if !jsonOutput {
				stepsLabel := "server default (16)"
				if maxSteps > 0 {
					stepsLabel = fmt.Sprintf("%d", maxSteps)
				}
				headerLabel := "(none)"
				if len(headers) > 0 {
					headerLabel = strings.Join(sortedKeys(headers), ", ")
				}
				fmt.Println(styleSection.Render("── autoconfig run ──"))
				fmt.Printf("  %s %s\n", styleKey.Render("Feed:    "), targetURL)
				fmt.Printf("  %s %s\n", styleKey.Render("Provider:"), providerType)
				fmt.Printf("  %s %s\n", styleKey.Render("Model:   "), modelName)
				fmt.Printf("  %s %s\n", styleKey.Render("Headers: "), styleDim.Render(headerLabel))
				fmt.Printf("  %s %s\n", styleKey.Render("Steps:   "), styleDim.Render(stepsLabel))
			}

			if !yes && !jsonOutput {
				confirm := true
				flushStdin()
				if err := huh.NewConfirm().
					Title("Run autoconfig with these settings?").
					Affirmative("Yes, run").
					Negative("Cancel").
					Value(&confirm).
					WithTheme(dlkPromptTheme).Run(); err != nil || !confirm {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			req := &protos.AutoConfigFeedRequest{
				Url:      targetURL,
				Headers:  headers,
				Provider: provider,
				Model:    model,
				MaxSteps: int32(maxSteps),
				Verbose:  verbose,
			}

			var done *protos.AutoConfigFeedEvent
			err = client.AutoConfigFeed(req, func(ev *protos.AutoConfigFeedEvent) {
				switch ev.Kind {
				case protos.AutoConfigEventKind_STEP:
					if !jsonOutput {
						fmt.Printf("  %s %s %s\n", styleDim.Render(fmt.Sprintf("%2d", ev.Step)),
							styleKey.Render(ev.Tool), styleDim.Render(ev.Detail))
					}
				case protos.AutoConfigEventKind_LLM_IO:
					if !jsonOutput {
						printLLMIO(int(ev.Step), ev.LlmPrompt, ev.LlmResponse)
					}
				case protos.AutoConfigEventKind_DONE:
					done = ev
				case protos.AutoConfigEventKind_ERROR:
					fmt.Printf("%s %s\n", styleErr.Render("✗"), ev.Detail)
				}
			})
			if err != nil {
				return fmt.Errorf("autoconfig: %w", err)
			}
			if done == nil {
				return fmt.Errorf("agent finished without producing a config")
			}

			if jsonOutput {
				return printJSON(map[string]any{
					"config_yaml": done.FeedConfigYaml,
					"summary":     done.Summary,
					"confidence":  done.Confidence,
				})
			}

			confStyle := styleErr
			switch {
			case done.Confidence >= 0.8:
				confStyle = styleOK
			case done.Confidence >= 0.5:
				confStyle = styleWarn
			}
			fmt.Printf("\n%s %s\n", styleBold.Render("Confidence:"), confStyle.Render(fmt.Sprintf("%.2f", done.Confidence)))
			if done.Summary != "" {
				fmt.Printf("%s %s\n", styleKey.Render("Rationale:"), done.Summary)
			}
			fmt.Println(styleSection.Render("── feed config ──"))
			fmt.Println(done.FeedConfigYaml)

			if updateFile != "" {
				return mergeAutoConfig(updateFile, done.FeedConfigYaml, yes)
			}
			fmt.Printf("%s paste into your feeds.yml, then `dlk feeds apply <file>`\n", styleKey.Render("Next:"))
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&headerFlags, "header", "H", nil, "Seed HTTP header \"Key: Value\" (repeatable)")
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "LLM provider (type or configured profile name)")
	cmd.Flags().StringVarP(&model, "model", "m", "", "LLM model override")
	cmd.Flags().IntVar(&maxSteps, "max-steps", 0, "Cap on agent tool calls (0 = server default)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Stream the raw LLM prompt and response for each agent turn")
	cmd.Flags().StringVar(&updateFile, "update", "", "Merge the discovered config into this feeds YAML file (shows a diff for existing feeds)")
	return cmd
}

// printLLMIO renders one agent turn's raw LLM prompt and response. The response is
// pretty-printed as JSON when it parses (the model replies with a JSON action),
// otherwise shown raw.
func printLLMIO(turn int, prompt, response string) {
	fmt.Printf("\n%s\n", styleSection.Render(fmt.Sprintf("── LLM turn %d ──", turn)))
	fmt.Println(styleKey.Render("input:"))
	fmt.Println(styleDim.Render(strings.TrimSpace(prompt)))
	fmt.Println(styleKey.Render("output:"))
	var buf bytes.Buffer
	if json.Indent(&buf, []byte(strings.TrimSpace(response)), "", "  ") == nil {
		fmt.Println(buf.String())
	} else {
		fmt.Println(strings.TrimSpace(response))
	}
}

// resolveFeedURL inspects rawURL and returns the URL autoconfig should run
// against. When rawURL is already a feed it is returned unchanged. When it is an
// HTML landing page, the discovered feeds are presented (interactively, unless
// yes/json forces the first) and the chosen one is returned. It errors when the
// page yields no feeds or the body is not usable.
func resolveFeedURL(client *downlinkclient.DownlinkClient, rawURL string, headers map[string]string, yes bool) (string, error) {
	resp, err := client.InspectFeed(rawURL, headers, 5)
	if err != nil {
		return "", fmt.Errorf("inspect feed: %w", err)
	}
	diag := resp.GetDiagnosis()

	switch diag.GetFeedTypeGuess() {
	case "rss", "atom", "json-feed":
		return rawURL, nil
	case "html":
		// fall through to discovery
	default:
		return "", fmt.Errorf("not a usable feed: %s", diag.GetVerdict())
	}

	discovered := diag.GetDiscoveredFeeds()
	if len(discovered) == 0 {
		return "", fmt.Errorf("no RSS/Atom feeds found on %s — pass a direct feed URL", rawURL)
	}

	if !jsonOutput {
		fmt.Printf("%s %s is a web page, not a feed. Found %d feed(s):\n",
			styleWarn.Render("!"), rawURL, len(discovered))
	}

	if yes || jsonOutput || len(discovered) == 1 {
		chosen := discovered[0]
		if !jsonOutput {
			fmt.Printf("  %s %s\n", styleKey.Render("Using:"), chosen)
		}
		return chosen, nil
	}

	chosen := discovered[0]
	opts := make([]huh.Option[string], 0, len(discovered))
	for _, u := range discovered {
		opts = append(opts, huh.NewOption(u, u))
	}
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Which feed do you want to configure?").
		Options(opts...).
		Value(&chosen).
		WithTheme(dlkPromptTheme).Run(); err != nil {
		return "", fmt.Errorf("cancelled")
	}
	return chosen, nil
}

// mergeAutoConfig merges the single-feed config YAML emitted by autoconfig into
// the feeds file at path. When a feed with the same URL already exists, its
// scraper block is replaced (identity fields kept) and a field-level diff is
// shown; otherwise the feed is appended. With yes, the confirmation is skipped.
func mergeAutoConfig(path, configYAML string, yes bool) error {
	var parsed models.FeedsFile
	if err := yaml.Unmarshal([]byte(configYAML), &parsed); err != nil {
		return fmt.Errorf("parse generated config: %w", err)
	}
	if len(parsed.Feeds) == 0 {
		return fmt.Errorf("generated config has no feed entry")
	}
	newFeed := parsed.Feeds[0]

	ff, err := loadFeedsFile(path)
	if err != nil {
		return err
	}

	idx := -1
	for i, f := range ff.Feeds {
		if f.URL == newFeed.URL {
			idx = i
			break
		}
	}

	if idx < 0 {
		ff.Feeds = append(ff.Feeds, newFeed)
		if !confirmMerge(fmt.Sprintf("Add new feed %s to %s?", newFeed.URL, path), yes) {
			fmt.Println("Cancelled.")
			return nil
		}
		fmt.Printf("%s adding new feed to %s\n", styleKey.Render("Update:"), path)
		return writeFeedsFile(path, ff)
	}

	existing := ff.Feeds[idx]
	changes := diffScraper(existing.Scraper, newFeed.Scraper)
	if len(changes) == 0 {
		fmt.Printf("%s %s already matches the generated config; nothing to update\n",
			styleOK.Render("✓"), newFeed.URL)
		return nil
	}

	fmt.Println(styleSection.Render("── changes ──"))
	for _, c := range changes {
		fmt.Printf("  %s\n", c)
	}
	if !confirmMerge(fmt.Sprintf("Apply these changes to %s in %s?", newFeed.URL, path), yes) {
		fmt.Println("Cancelled.")
		return nil
	}

	// Keep identity fields, replace the scraper block.
	existing.Scraper = newFeed.Scraper
	ff.Feeds[idx] = existing
	fmt.Printf("%s updated %s in %s\n", styleKey.Render("Update:"), newFeed.URL, path)
	return writeFeedsFile(path, ff)
}

// confirmMerge prompts for a yes/no confirmation, returning true to proceed.
// It returns true immediately when yes (or jsonOutput) is set.
func confirmMerge(title string, yes bool) bool {
	if yes || jsonOutput {
		return true
	}
	confirm := true
	flushStdin()
	if err := huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("Cancel").
		Value(&confirm).
		WithTheme(dlkPromptTheme).Run(); err != nil {
		return false
	}
	return confirm
}

// writeFeedsFile marshals a feeds file back to YAML and writes it to path.
func writeFeedsFile(path string, ff *models.FeedsFile) error {
	out, err := yaml.Marshal(ff)
	if err != nil {
		return fmt.Errorf("marshal feeds file: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("%s %s, then `dlk feeds apply %s`\n", styleKey.Render("Next:"), "review the file", path)
	return nil
}

// diffScraper returns human-readable "key: old → new" lines for the fields that
// differ between two scraper configs. Empty when they are equivalent.
func diffScraper(old, new models.ScraperConfig) []string {
	var out []string
	add := func(key, o, n string) {
		if o != n {
			out = append(out, fmt.Sprintf("%s %s → %s",
				styleKey.Render(key+":"), styleDim.Render(quoteEmpty(o)), quoteEmpty(n)))
		}
	}
	add("type", old.Type, new.Type)
	add("scraping", old.Scraping, new.Scraping)
	add("selectors.article", selVal(old.Selectors, "article"), selVal(new.Selectors, "article"))
	add("selectors.cutoff", selVal(old.Selectors, "cutoff"), selVal(new.Selectors, "cutoff"))
	add("selectors.blacklist", selVal(old.Selectors, "blacklist"), selVal(new.Selectors, "blacklist"))

	for _, k := range sortedHeaderKeys(old.Headers, new.Headers) {
		add("headers."+k, old.Headers[k], new.Headers[k])
	}
	return out
}

// selVal safely reads a named field from a possibly-nil Selectors.
func selVal(s *models.Selectors, field string) string {
	if s == nil {
		return ""
	}
	switch field {
	case "article":
		return s.Article
	case "cutoff":
		return s.Cutoff
	case "blacklist":
		return s.Blacklist
	}
	return ""
}

// sortedHeaderKeys returns the union of two header maps' keys, sorted.
func sortedHeaderKeys(a, b map[string]string) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// quoteEmpty renders an empty value as "(none)" so a diff line stays readable.
func quoteEmpty(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

// ── inspect ───────────────────────────────────────────────────────────────────

func newInspectCmd() *cobra.Command {
	var headerFlags []string
	cmd := &cobra.Command{
		Use:   "inspect <rss-url>",
		Short: "Inspect a feed URL and scaffold a starter config",
		Long: `Fetch and inspect a feed URL, then print what came back — diagnosis, sample
article links, and a starter feed configuration to build on.

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
				Enabled: true,
				Scraper: models.ScraperConfig{
					Type:    feedType,
					Headers: headers,
				},
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
