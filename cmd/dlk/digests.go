package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ma111e/downlink/pkg/digestlayouts"
	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"github.com/ma111e/downlink/pkg/utils"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

var (

	// Digest flags
	digestLimit int
)

// Digest commands
func createDigestCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "digest",
		Short: "Manage article digests",
		Long:  `Create and view article digests.`,
	}

	// List digests command
	var listThemes bool
	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List digests",
		Long:    `View all available digests.`,
		Run: func(cmd *cobra.Command, args []string) {
			if listThemes {
				fmt.Println("Available layout themes:")
				for _, l := range digestlayouts.All() {
					fmt.Printf("  %-12s %s\n", l.Name, l.Description)
				}
				return
			}

			client := getNewDownlinkClient()

			// --json dumps full digest data; the table view only needs summaries.
			var digests []models.Digest
			var err error
			if jsonOutput {
				digests, err = client.ListDigestsFull(digestLimit)
			} else {
				digests, err = client.ListDigests(digestLimit)
			}
			if err != nil {
				fmt.Printf("Failed to list digests: %v\n", err)
				return
			}

			if len(digests) == 0 {
				fmt.Println("No digests found")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(digests, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printDigestTable(digests)
			}
		},
	}

	// Add flags for list command
	listCmd.Flags().IntVar(&digestLimit, "limit", 0, "Maximum number of digests to return (0 = all)")
	listCmd.Flags().BoolVar(&listThemes, "themes", false, "List available layout themes and exit")

	// Get digest command
	var showMarkdown bool
	getCmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get digest details",
		Long:  `View details of a specific digest. Omit ID to pick interactively.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			var digestId string
			if len(args) == 1 {
				digestId = args[0]
			} else {
				digest, err := selectDigest(client)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				if digest.Id == "" {
					fmt.Println("Cancelled.")
					return
				}
				digestId = digest.Id
			}

			digest, err := client.GetDigest(digestId)
			if err != nil {
				fmt.Printf("Failed to get digest: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(digest, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				articles, err := client.GetDigestArticles(digestId)
				if err != nil {
					fmt.Printf("Failed to get digest articles: %v\n", err)
					return
				}
				if showMarkdown {
					printDigestDetailMarkdown(digest, articles)
				} else {
					printDigestDetail(digest, articles)
				}
			}
		},
	}
	getCmd.Flags().BoolVar(&showMarkdown, "markdown", false, "Display summary in styled markdown format")

	// Generate digest command
	var digestFrom, digestTo, digestBetween, digestDay, digestLayout, digestTestID string
	var digestProvider, digestModel string
	var digestDryRun, digestRefreshFeeds, digestTest, digestNoGHPages, digestGHPages, digestReanalyzeOnModelChange, digestReanalyze, digestVibeScore, digestGlossary, digestSelectModel bool
	var digestStandardSynthesis, digestComprehensiveSynthesis, digestExecutiveSummary bool
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate a new digest",
		Long: `Create a new digest from recent articles.

Examples:
  downlink-cli digest generate                      # Last 24 hours (default)
  downlink-cli digest generate --from 7d             # Last 7 days
  downlink-cli digest generate --from 2h             # Last 2 hours
  downlink-cli digest generate --from 2025-01-01    # From specific date
  downlink-cli digest generate --from 7d --to 1d    # Between 7 days and 1 day ago
  downlink-cli digest generate --day 2025-01-15     # Single UTC day (midnight to midnight)
  downlink-cli digest generate --day yesterday      # Yesterday in UTC`,
		Run: func(cmd *cobra.Command, args []string) {
			if digestGHPages && digestNoGHPages {
				fmt.Println("Error: --gh-pages and --no-gh-pages are mutually exclusive")
				os.Exit(1)
			}

			// --test-digest-id implies --test: choosing a specific stored digest to send
			// only makes sense in test mode, so passing the ID alone enables it rather
			// than silently falling through to normal generation over the time window.
			if digestTestID != "" {
				digestTest = true
			}

			client := getNewDownlinkClient()

			if digestTest {
				if !digestlayouts.Valid(digestLayout) {
					fmt.Printf("Unknown layout theme %q. Run 'digest list --themes' to see available themes.\n", digestLayout)
					return
				}

				prog := newBatchProgress()
				prog.addHiddenRow("notify", "sending test digest")
				prog.startSpinner()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				sigCh := make(chan os.Signal, 2)
				signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
				go func() {
					select {
					case <-ctx.Done():
						return
					case <-sigCh:
						prog.updateRow("cancel", "cancelling...")
						cancel()
						select {
						case <-sigCh:
							os.Exit(130)
						case <-ctx.Done():
						}
					}
				}()
				defer signal.Stop(sigCh)

				handler := newDigestProgressHandler(prog)
				digest, err := client.GenerateDigestWithOptions(ctx, downlinkclient.GenerateDigestOptions{
					StartTime:    time.Now(),
					EndTime:      time.Now(),
					Layout:       digestLayout,
					Test:         true,
					TestDigestID: digestTestID,
					OnEvent:      handler,
				})

				prog.stop()

				if err != nil {
					if ctx.Err() != nil {
						fmt.Println("\nDigest notification test cancelled (confirmed by server).")
						return
					}
					fmt.Printf("Failed to send test digest: %v\n", err)
					return
				}

				fmt.Printf("\nTest digest sent: %s\n", digest.Id)
				articleCount := 0
				if digest.ArticleCount != nil {
					articleCount = *digest.ArticleCount
				}
				fmt.Printf("Contains %d articles\n", articleCount)
				return
			}

			var fromTime *time.Time
			var toTimeVal time.Time

			if digestDay != "" {
				if digestFrom != "" || digestTo != "" || digestBetween != "" {
					fmt.Println("Error: --day cannot be combined with --from, --to, or --between")
					return
				}
				start, end, err := utils.ParseDayUTC(digestDay)
				if err != nil {
					fmt.Println(err)
					return
				}
				fromTime = &start
				toTimeVal = end
			} else {
				defaultFrom := time.Now().Add(-24 * time.Hour)
				ft, toTime, err := parseTimeWindow(digestFrom, digestTo, digestBetween, &defaultFrom)
				if err != nil {
					fmt.Println(err)
					return
				}
				fromTime = ft
				if toTime != nil {
					toTimeVal = *toTime
				} else {
					toTimeVal = time.Now()
				}
			}

			// Calculate hours from time window
			hours := int(toTimeVal.Sub(*fromTime).Hours())
			if hours < 1 {
				hours = 1 // Minimum 1 hour
			}

			// Refresh feeds over the same window before generating the digest, when
			// requested. Skipped in dry-run to keep that mode side-effect-free.
			if digestRefreshFeeds {
				if digestDryRun {
					fmt.Println("(skipping feed refresh in --dry-run mode)")
				} else {
					fmt.Printf("Refreshing feeds from %s to %s...\n",
						fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
					if err := refreshAllFeedsWithWindow(client, fromTime, &toTimeVal, false, false, 0); err != nil {
						fmt.Println(err)
						return
					}
					fmt.Println()
				}
			}

			// Dry-run mode: just list articles that would be included
			if digestDryRun {
				excludeDigested, _ := cmd.Flags().GetBool("exclude-digested")
				filter := models.ArticleFilter{
					StartDate:       fromTime,
					EndDate:         &toTimeVal,
					ExcludeDigested: excludeDigested,
					// Match the real digest fetch: the whole window, not a UI page.
					Unbounded: true,
				}
				articles, err := client.ListArticles(filter)
				if err != nil {
					fmt.Printf("Failed to list articles: %v\n", err)
					return
				}

				fmt.Printf("Would generate digest with %d articles from %s to %s:\n\n",
					len(articles), fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
				if len(articles) == 0 {
					fmt.Println("No articles found in this time window.")
				} else {
					printArticleTable(articles)
				}
				fmt.Println("\n(no digest generated)")
				return
			}

			if !digestlayouts.Valid(digestLayout) {
				fmt.Printf("Unknown layout theme %q. Run 'digest list --themes' to see available themes.\n", digestLayout)
				return
			}

			// Interactively pick a model when --select-model is set.
			if digestSelectModel {
				pt, mn, err := selectModelInteractive(client, digestProvider)
				if err != nil {
					fmt.Printf("%v\n", err)
					return
				}
				if mn == "" {
					fmt.Println("Cancelled.")
					return
				}
				digestProvider = pt
				digestModel = mn
			}

			skipAnalysis, _ := cmd.Flags().GetBool("skip-analysis")
			skipDuplicates, _ := cmd.Flags().GetBool("skip-duplicates")
			excludeDigested, _ := cmd.Flags().GetBool("exclude-digested")
			oneShotAnalysis, _ := cmd.Flags().GetBool("one-shot")

			// Tri-state override for the opt-in executive summary, same semantics as
			// vibe_score: only override the server config when the flag was set.
			var executiveSummary *bool
			if cmd.Flags().Changed("executive-summary") {
				executiveSummary = &digestExecutiveSummary
			}

			if skipLLM, _ := cmd.Flags().GetBool("skip-llm"); skipLLM {
				skipAnalysis = true
				skipDuplicates = true
			}

			prog := newBatchProgress()
			prog.addRow("fetch", "fetching articles")
			if !skipAnalysis {
				prog.addRow("analyze", "analyzing articles")
			}
			if !skipDuplicates {
				prog.addHiddenRow("dedupe", "deduplicating")
			}
			// The executive summary is opt-in: register its progress row unless it was
			// explicitly disabled for this run. The row stays hidden until the
			// "summarize" stage actually runs.
			if executiveSummary == nil || *executiveSummary {
				prog.addHiddenRow("summarize", "generating summary")
			}
			prog.addHiddenRow("store", "storing digest")
			// The glossary is opt-in (server config or --glossary): register its row unless it
			// was explicitly disabled for this run. The row stays hidden until the "glossary"
			// stage actually runs.
			if !(cmd.Flags().Changed("glossary") && !digestGlossary) {
				prog.addHiddenRow("glossary", "building glossary")
			}
			prog.addHiddenRow("notify", "sending notification")
			prog.startSpinner()

			// Allow the user to cancel digest generation with Ctrl-C. The first
			// SIGINT signals the server to stop at the next stage boundary and
			// waits for its "cancelled" confirmation before exiting. A second
			// SIGINT forces an immediate exit.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sigCh := make(chan os.Signal, 2)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			go func() {
				select {
				case <-ctx.Done():
					return
				case <-sigCh:
					prog.updateRow("cancel", "cancelling...")
					cancel()
					select {
					case <-sigCh:
						os.Exit(130)
					case <-ctx.Done():
					}
				}
			}()
			defer signal.Stop(sigCh)

			var ghPagesEnabled *bool
			if digestNoGHPages {
				v := false
				ghPagesEnabled = &v
			} else if digestGHPages {
				v := true
				ghPagesEnabled = &v
			}

			// Tri-state: only override the server's vibe_score config when the
			// flag was explicitly set; otherwise leave nil to use the server default.
			var vibeScore *bool
			if cmd.Flags().Changed("vibe-score") {
				vibeScore = &digestVibeScore
			}

			// Tri-state override for glossary mode, same semantics as vibe_score.
			var glossary *bool
			if cmd.Flags().Changed("glossary") {
				glossary = &digestGlossary
			}

			// Tri-state overrides for the Standard and Comprehensive article
			// summaries, same semantics as vibe_score.
			var standardSynthesis *bool
			if cmd.Flags().Changed("standard-synthesis") {
				standardSynthesis = &digestStandardSynthesis
			}
			var comprehensiveSynthesis *bool
			if cmd.Flags().Changed("comprehensive-synthesis") {
				comprehensiveSynthesis = &digestComprehensiveSynthesis
			}

			handler := newDigestProgressHandler(prog)
			digest, err := client.GenerateDigestWithOptions(ctx, downlinkclient.GenerateDigestOptions{
				StartTime:              *fromTime,
				EndTime:                toTimeVal,
				SkipAnalysis:           skipAnalysis,
				SkipDuplicates:         skipDuplicates,
				ExcludeDigested:        excludeDigested,
				Layout:                 digestLayout,
				OneShotAnalysis:        oneShotAnalysis,
				GHPagesEnabled:         ghPagesEnabled,
				ReanalyzeOnModelChange: digestReanalyzeOnModelChange,
				Reanalyze:              digestReanalyze,
				VibeScore:              vibeScore,
				Glossary:               glossary,
				StandardSynthesis:      standardSynthesis,
				ComprehensiveSynthesis: comprehensiveSynthesis,
				ExecutiveSummary:       executiveSummary,
				Provider:               digestProvider,
				Model:                  digestModel,
				OnEvent:                handler,
			})

			prog.stop()

			if err != nil {
				if ctx.Err() != nil {
					fmt.Println("\nDigest generation cancelled (confirmed by server).")
					return
				}
				fmt.Printf("Failed to generate digest: %v\n", err)
				return
			}

			fmt.Printf("\nNew digest generated: %s\n", digest.Id)
			articleCount := 0
			if digest.ArticleCount != nil {
				articleCount = *digest.ArticleCount
			}
			fmt.Printf("Contains %d articles from %s to %s\n",
				articleCount, fromTime.Format("2006-01-02 15:04:05"), toTimeVal.Format("2006-01-02 15:04:05"))
		},
	}

	// Add flags for generate command
	generateCmd.Flags().StringVar(&digestFrom, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '24h'; default: 24h)")
	generateCmd.Flags().StringVar(&digestTo, "to", "", "End of time window (e.g., 'now', '2025-01-01', '1h')")
	generateCmd.Flags().StringVar(&digestBetween, "between", "", "Filter articles between two dates/durations (e.g., '7d,1d', '2025-01-01,2025-01-07')")
	generateCmd.Flags().StringVar(&digestDay, "day", "", "Select a single day, midnight-to-midnight UTC (YYYY-MM-DD, 'today', or 'yesterday'). Mutually exclusive with --from/--to/--between")
	generateCmd.Flags().BoolVar(&digestDryRun, "dry-run", false, "List matching articles without generating digest")
	generateCmd.Flags().BoolVar(&digestRefreshFeeds, "refresh-feeds", false, "Refresh all feeds over the same time window before generating the digest")
	generateCmd.Flags().Bool("skip-llm", false, "Skip all LLM usage (analysis, duplicate detection, and summary)")
	generateCmd.Flags().Bool("skip-analysis", false, "Skip LLM-based article analysis")
	generateCmd.Flags().Bool("skip-duplicates", false, "Skip LLM-based duplicate detection")
	generateCmd.Flags().BoolVar(&digestExecutiveSummary, "executive-summary", false, "Generate the digest-level executive summary for this run [overrides server config; use --executive-summary=false to force off]")
	generateCmd.Flags().Bool("one-shot", false, "Analyze missing articles with one full LLM prompt instead of the multi-step chain")
	generateCmd.Flags().Bool("exclude-digested", false, "Exclude articles already included in a previous digest")
	generateCmd.Flags().StringVar(&digestLayout, "theme", "default", "Layout/graphical theme for the digest (see: digest list --themes)")
	generateCmd.Flags().StringVarP(&digestProvider, "provider", "p", "", "Provider override for this run (a provider type or a configured profile name, auto-detected by the server); applies to all LLM steps")
	generateCmd.Flags().StringVarP(&digestModel, "model", "m", "", "Model override for this run. If given without --provider, the server finds the provider offering it (errors if ambiguous)")
	generateCmd.Flags().BoolVar(&digestSelectModel, "select-model", false, "Interactively pick a model; lists every provider's models (or just --provider's when set)")
	generateCmd.Flags().BoolVar(&digestTest, "test", false, "Send a stored test digest to configured notification channels without generating a new digest")
	generateCmd.Flags().StringVar(&digestTestID, "test-digest-id", "", "Digest ID to send as a test (implies --test; default: server-selected rich test digest)")
	generateCmd.Flags().BoolVar(&digestNoGHPages, "no-gh-pages", false, "Disable GitHub Pages publishing for this run (overrides server config)")
	generateCmd.Flags().BoolVar(&digestGHPages, "gh-pages", false, "Enable GitHub Pages publishing for this run (overrides server config)")
	generateCmd.Flags().BoolVar(&digestReanalyzeOnModelChange, "reanalyze-on-model-change", false, "Re-analyze articles whose existing analysis was produced by a different model than the one currently configured")
	generateCmd.Flags().BoolVar(&digestReanalyze, "reanalyze", false, "Re-analyze every article in the window, even if it already has an analysis")
	generateCmd.Flags().BoolVar(&digestVibeScore, "vibe-score", false, "Use the legacy single-number LLM importance prompt instead of the rubric scoring system for this run [overrides server config; use --vibe-score=false to force the rubric]")
	generateCmd.Flags().BoolVar(&digestGlossary, "glossary", false, "Generate glossary-mode content (plain-language explanation + jargon glossary) for this run [overrides server config; use --glossary=false to force off]")
	generateCmd.Flags().BoolVar(&digestStandardSynthesis, "standard-synthesis", false, "Generate the Standard article summary for this run [overrides server config; use --standard-synthesis=false to force off]")
	generateCmd.Flags().BoolVar(&digestComprehensiveSynthesis, "comprehensive-synthesis", false, "Generate the Full (comprehensive) article summary for this run [overrides server config; use --comprehensive-synthesis=false to force off]")

	// Get digest articles command
	articlesCmd := &cobra.Command{
		Use:   "articles [digest-id]",
		Short: "List articles in digest",
		Long:  `View all articles included in a specific digest. Omit ID to pick interactively.`,
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			var digestId string
			if len(args) == 1 {
				digestId = args[0]
			} else {
				digest, err := selectDigest(client)
				if err != nil {
					fmt.Printf("Error: %v\n", err)
					return
				}
				if digest.Id == "" {
					fmt.Println("Cancelled.")
					return
				}
				digestId = digest.Id
			}

			articles, err := client.GetDigestArticles(digestId)
			if err != nil {
				fmt.Printf("Failed to get digest articles: %v\n", err)
				return
			}

			if len(articles) == 0 {
				fmt.Println("No articles in this digest.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(articles, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Printf("Articles in digest %s:\n\n", digestId)
				printArticleTable(articles)
			}
		},
	}

	cmd.AddCommand(listCmd, getCmd, generateCmd, articlesCmd)
	return cmd
}

// newDigestProgressHandler returns a stateful event handler that maps server-sent
// DigestProgressEvents to batchProgress rows. With parallel article analysis,
// each article gets its own row keyed by article_id; task events update that
// row with the currently-running task. The "analyze" header row summarizes
// total progress across all in-flight articles.
func newDigestProgressHandler(prog *batchProgress) func(*protos.DigestProgressEvent) {
	var (
		total     uint32
		started   int
		completed int
		// Ordered list of article IDs as they begin, so new article rows
		// are inserted in deterministic order beneath the "analyze" header.
		articleOrder     []string
		articleSeen      = map[string]bool{}
		articleCompleted = map[string]bool{}
	)

	articleRowId := func(articleId string) string { return "analyze:art:" + articleId }
	// Row format: "      └ <title> · <task> (N/M)" (~8 chars of indent/glyph).
	// Renderer caps labels at 100 chars; reserve ~22 for the task suffix.
	shortTitle := func(t string) string {
		t = strings.TrimSpace(t)
		if t == "" {
			return "(untitled)"
		}
		const maxTitle = 70
		if len(t) > maxTitle {
			t = t[:maxTitle-1] + "…"
		}
		return t
	}

	ensureArticleStarted := func(articleId, articleTitle string) {
		if articleSeen[articleId] {
			return
		}
		articleSeen[articleId] = true
		articleOrder = append(articleOrder, articleId)
		started++
		afterKey := "analyze"
		if len(articleOrder) > 1 {
			afterKey = articleRowId(articleOrder[len(articleOrder)-2])
		}
		prog.insertRowAfter(afterKey, articleRowId(articleId),
			fmt.Sprintf("      └ %s", shortTitle(articleTitle)))
	}

	markArticleCompleted := func(articleId string) bool {
		if articleCompleted[articleId] {
			return false
		}
		articleCompleted[articleId] = true
		completed++
		return true
	}

	updateAnalyzeHeader := func() {
		var label string
		if total > 0 {
			inFlight := started - completed
			if inFlight < 0 {
				inFlight = 0
			}
			label = fmt.Sprintf("analyzing [%d/%d, %d in parallel]", completed, total, inFlight)
		} else {
			label = "analyzing articles"
		}
		prog.updateRow("analyze", label)
	}

	return func(ev *protos.DigestProgressEvent) {
		switch ev.Stage {
		case "fetch":
			if ev.Total > 0 {
				prog.completeRow("fetch", true, fmt.Sprintf("%d articles", ev.Total))
			} else {
				prog.updateRow("fetch", "fetching articles...")
			}
		case "analyze":
			if ev.Total > 0 {
				total = ev.Total
			}
			if ev.ArticleId != "" {
				ensureArticleStarted(ev.ArticleId, ev.ArticleTitle)
			}
			updateAnalyzeHeader()
		case "analyze_task":
			if ev.ArticleId == "" {
				return
			}
			rowId := articleRowId(ev.ArticleId)
			title := shortTitle(ev.ArticleTitle)
			switch ev.TaskStatus {
			case "started":
				prog.updateRow(rowId,
					fmt.Sprintf("      └ %s · %s (%d/%d)", title, ev.TaskName, ev.TaskIndex, ev.TaskTotal))
			case "completed":
				if ev.TaskIndex == ev.TaskTotal {
					// Last task for this article: mark the row done.
					prog.updateRow(rowId, fmt.Sprintf("      └ %s", title))
					prog.completeRow(rowId, true, "")
					if markArticleCompleted(ev.ArticleId) {
						updateAnalyzeHeader()
						if completed == int(total) && total > 0 {
							prog.completeRow("analyze", true, fmt.Sprintf("%d articles", total))
						}
					}
				} else {
					prog.updateRow(rowId,
						fmt.Sprintf("      └ %s · %s done", title, ev.TaskName))
				}
			case "error":
				ensureArticleStarted(ev.ArticleId, ev.ArticleTitle)
				prog.completeRow(rowId, false, fmt.Sprintf("%s: %s", title, ev.Error))
				if markArticleCompleted(ev.ArticleId) {
					updateAnalyzeHeader()
				}
			}
		case "dedupe":
			prog.showRow("dedupe")
			if ev.Message != "" && ev.Message != "identifying duplicate articles..." {
				prog.completeRow("dedupe", true, ev.Message)
			}
		case "summarize":
			prog.showRow("summarize")
			if ev.Message == "digest summary generated" {
				prog.completeRow("summarize", true, "done")
			}
		case "store":
			prog.showRow("store")
		case "glossary":
			prog.showRow("glossary")
			if strings.HasPrefix(ev.Message, "glossary built") {
				prog.completeRow("glossary", true, ev.Message)
			}
		case "notify":
			prog.showRow("notify")
		case "done":
			for _, id := range []string{"fetch", "analyze", "dedupe", "summarize", "store", "glossary", "notify"} {
				prog.completeRow(id, true, "")
			}
		case "error":
			prog.completeRow("fetch", false, ev.Error)
			prog.completeRow("analyze", false, ev.Error)
			prog.completeRow("dedupe", false, ev.Error)
			prog.completeRow("summarize", false, ev.Error)
			prog.completeRow("store", false, ev.Error)
			prog.completeRow("glossary", false, ev.Error)
			prog.completeRow("notify", false, ev.Error)
		}
	}
}
