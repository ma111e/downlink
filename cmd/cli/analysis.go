package main

import (
	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	// Analysis flags
	persona      string
	providerName string
	providerType string
	modelName    string
	profileNames []string
)

// Analysis commands
func createAnalysisCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analysis",
		Short: "Manage article analysis",
		Long:  `View and run LLM-based analysis of articles.`,
	}

	// Get analysis config command
	getConfigCmd := &cobra.Command{
		Use:   "config",
		Short: "Get analysis configuration",
		Long:  `View the current configuration for article analysis.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			config, err := client.GetAnalysisConfig()
			if err != nil {
				fmt.Printf("Error getting analysis config: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(config, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printAnalysisConfig(config)
			}
		},
	}

	// Update analysis config command
	updateConfigCmd := &cobra.Command{
		Use:   "set",
		Short: "Update analysis configuration",
		Long:  `Set the configuration for article analysis.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if providerName == "" {
				fmt.Println("Error: Provider name is required")
				return
			}

			config := models.AnalysisConfig{
				Provider: providerName,
				Persona:  persona,
			}

			err := client.UpdateAnalysisConfig(config)
			if err != nil {
				fmt.Printf("Failed to update analysis config: %v\n", err)
				return
			}

			fmt.Println("Analysis configuration updated successfully")
		},
	}

	// Add flags for update config command
	updateConfigCmd.Flags().StringVarP(&persona, "persona", "P", "", "Persona to use for analysis")
	updateConfigCmd.Flags().StringVarP(&providerName, "provider-name", "n", "", "Name of the configured provider to use (required)")
	updateConfigCmd.MarkFlagRequired("provider-name")

	// Analyze article(s) command — smart handling of single vs batch
	var runFrom, runTo, runBetween string
	var runDryRun, runKeyPointsOnly, runSelectModel, runAllTime bool
	analyzeCmd := &cobra.Command{
		Use:   "run [article-id|feed-id-or-name|all]",
		Short: "Analyze article(s)",
		Long: `Analyze one or more articles.

Single Article Analysis:
  downlink-cli analysis run abc123                           # Analyze article by ID
  downlink-cli analysis run abc123 --key-points-only         # Extract key points only
  downlink-cli analysis run abc123 --provider openai --select-model  # Interactive model selection

Batch Analysis by Feed/Time:
  downlink-cli analysis run                        # Analyze all articles
  downlink-cli analysis run my-feed                # Analyze articles from a feed
  downlink-cli analysis run all                    # Explicitly analyze all articles
  downlink-cli analysis run --from -7d             # Analyze articles from last 7 days
  downlink-cli analysis run my-feed --from -1d    # Analyze feed articles from last 24h
  downlink-cli analysis run --from -7d --key-points-only # Key points only for last 7 days
  downlink-cli analysis run --dry-run --from -7d  # Preview matching articles`,
		Args: cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			// Validate: --profile cannot be combined with --provider/--model/--select-model
			if len(profileNames) > 0 && (providerType != "" || modelName != "" || runSelectModel) {
				fmt.Println("Error: --profile cannot be combined with --provider, --model, or --select-model")
				return
			}

			// Validate profile names against server-known providers
			if len(profileNames) > 0 {
				providers, err := client.GetLLMProviders()
				if err != nil {
					fmt.Printf("Failed to list profiles: %v\n", err)
					return
				}
				nameSet := make(map[string]bool, len(providers))
				for _, p := range providers {
					nameSet[strings.ToLower(p.Name)] = true
				}
				var unknowns []string
				for _, pn := range profileNames {
					if !nameSet[strings.ToLower(pn)] {
						unknowns = append(unknowns, pn)
					}
				}
				if len(unknowns) > 0 {
					fmt.Printf("Error: unknown profile(s): %s\n", strings.Join(unknowns, ", "))
					fmt.Println("Available profiles:")
					for _, p := range providers {
						enabled := ""
						if !p.Enabled {
							enabled = " (disabled)"
						}
						fmt.Printf("  - %s%s\n", p.Name, enabled)
					}
					return
				}
			}

			// Handle --select-model flag: fetch and let user choose from available models
			if runSelectModel {
				if providerType == "" {
					fmt.Println("Error: --provider is required when using --select-model")
					return
				}

				modelsResp, err := client.GetAvailableModels()
				if err != nil {
					fmt.Printf("Failed to get available models: %v\n", err)
					return
				}

				if modelsResp == nil || len(modelsResp.Models) == 0 {
					fmt.Printf("No models available for provider %s\n", providerType)
					return
				}

				// Filter models by provider type
				var matchingModels []models.ModelInfo
				for _, m := range modelsResp.Models {
					if m.ProviderType == providerType || strings.EqualFold(m.ProviderType, providerType) {
						matchingModels = append(matchingModels, m)
					}
				}

				if len(matchingModels) == 0 {
					fmt.Printf("No models available for provider '%s'\n", providerType)
					fmt.Printf("Available providers: ")
					providersMap := make(map[string]bool)
					for _, m := range modelsResp.Models {
						providersMap[m.ProviderType] = true
					}
					for p := range providersMap {
						fmt.Printf("%s ", p)
					}
					fmt.Println()
					return
				}

				// Display models to user
				fmt.Printf("Available models for %s:\n\n", providerType)
				for i, m := range matchingModels {
					fmt.Printf("%d. %s", i+1, m.Name)
					if m.DisplayName != "" && m.DisplayName != m.Name {
						fmt.Printf(" (%s)", m.DisplayName)
					}
					if m.Description != "" {
						fmt.Printf("\n   %s", m.Description)
					}
					fmt.Println()
				}

				// Read user selection
				fmt.Print("\nSelect model number (1-" + fmt.Sprintf("%d", len(matchingModels)) + "): ")
				var choice int
				_, err = fmt.Scanln(&choice)
				if err != nil {
					fmt.Println("Error reading input")
					return
				}

				if choice < 1 || choice > len(matchingModels) {
					fmt.Println("Invalid selection")
					return
				}

				// Use selected model
				modelName = matchingModels[choice-1].Name
				fmt.Printf("\nSelected: %s\n\n", modelName)
			}

			// If a single article ID provided with no filters, treat as single-article mode
			if len(args) == 1 && runFrom == "" && runTo == "" && runBetween == "" && !runDryRun {
				articleId := args[0]

				// Track per-task progress rows keyed by task name
				prog := newBatchProgress()
				prog.startSpinner()

				onEvent := func(ev downlinkclient.AnalysisProgressEvent) {
					switch ev.Status {
					case "started":
						label := fmt.Sprintf("[%d/%d] %s", ev.TaskIndex, ev.TotalTasks, ev.TaskName)
						prog.addRow(ev.TaskName, label)
					case "completed":
						prog.completeRow(ev.TaskName, true, "done")
					case "error":
						summary := ev.Error
						if len(summary) > 40 {
							summary = summary[:37] + "..."
						}
						prog.completeRow(ev.TaskName, false, summary)
					}
				}

				analyzeWithProfile := func(profile string) bool {
					if profile != "" {
						fmt.Printf("\n=== profile: %s ===\n", profile)
					}
					var analysis models.ArticleAnalysis
					var err error
					if profile != "" {
						analysis, err = client.StreamAnalyzeArticleWithProfile(articleId, profile, runKeyPointsOnly, onEvent)
					} else if providerType != "" && modelName != "" {
						analysis, err = client.StreamAnalyzeArticleWithProviderModel(articleId, providerType, modelName, runKeyPointsOnly, onEvent)
					} else {
						analysis, err = client.StreamAnalyzeArticle(articleId, runKeyPointsOnly, onEvent)
					}

					prog.stop()

					if err != nil {
						fmt.Printf("Failed to analyze article: %v\n", err)
						return false
					}

					if jsonOutput {
						out, err := json.MarshalIndent(analysis, "", "  ")
						if err != nil {
							fmt.Printf("Error marshalling to JSON: %v\n", err)
							return false
						}
						fmt.Println(string(out))
					} else {
						printAnalysisDetail(analysis)
					}
					return true
				}

				if len(profileNames) > 0 {
					for _, pn := range profileNames {
						prog = newBatchProgress()
						prog.startSpinner()
						onEvent = func(ev downlinkclient.AnalysisProgressEvent) {
							switch ev.Status {
							case "started":
								label := fmt.Sprintf("[%d/%d] %s", ev.TaskIndex, ev.TotalTasks, ev.TaskName)
								prog.addRow(ev.TaskName, label)
							case "completed":
								prog.completeRow(ev.TaskName, true, "done")
							case "error":
								summary := ev.Error
								if len(summary) > 40 {
									summary = summary[:37] + "..."
								}
								prog.completeRow(ev.TaskName, false, summary)
							}
						}
						if !analyzeWithProfile(pn) {
							return
						}
					}
				} else {
					if !analyzeWithProfile("") {
						return
					}
				}
				return
			}

			// Batch mode: require a time window or explicit --all-time flag
			hasTimeFilter := runFrom != "" || runTo != "" || runBetween != ""
			if !hasTimeFilter && !runAllTime {
				fmt.Println("Error: batch analysis requires a time window (--from, --to, --between) or --all-time to analyze all articles regardless of date.")
				return
			}

			fromTime, toTime, err := parseTimeWindow(runFrom, runTo, runBetween, nil)
			if err != nil {
				fmt.Println(err)
				return
			}

			// Build article filter
			filter := models.ArticleFilter{}
			if fromTime != nil {
				filter.StartDate = fromTime
			}
			if toTime != nil {
				filter.EndDate = toTime
			}

			// Resolve feed argument
			if len(args) > 0 && args[0] != "all" {
				feedIdentifier := args[0]

				feed, feeds, err := findFeedByIDOrNormalizedName(client, feedIdentifier)
				if err != nil && feeds == nil {
					fmt.Printf("Failed to list feeds: %v\n", err)
					return
				}
				if err != nil {
					fmt.Println(err)
					printAvailableFeeds(feeds)
					return
				}

				filter.FeedId = feed.Id
			}

			// Fetch matching articles — page through all results when --all-time is set
			const pageSize = 100
			var allArticles []models.Article
			for {
				page, listErr := client.ListArticles(filter)
				if listErr != nil {
					fmt.Printf("Failed to list articles: %v\n", listErr)
					return
				}
				allArticles = append(allArticles, page...)
				if len(page) < pageSize || !runAllTime {
					break
				}
				filter.Offset += pageSize
			}

			if len(allArticles) == 0 {
				fmt.Println("No articles found matching filter.")
				return
			}

			if !runAllTime && len(allArticles) == pageSize {
				fmt.Println("Warning: 100 articles returned (server limit). Narrow your time window or use --all-time.")
			}

			// Dry-run mode: just list the articles
			if runDryRun {
				printArticleTable(allArticles)
				return
			}

			// Enqueue articles for analysis
			articleIds := make([]string, len(allArticles))
			for i, a := range allArticles {
				articleIds[i] = a.Id
			}

			if len(profileNames) > 0 {
				for _, pn := range profileNames {
					if err := client.EnqueueArticles(downlinkclient.EnqueueOptions{
						ArticleIds:   articleIds,
						ProviderName: pn,
						FastMode:     runKeyPointsOnly,
					}); err != nil {
						fmt.Printf("Failed to enqueue articles for profile %s: %v\n", pn, err)
						return
					}
				}
				total := len(allArticles) * len(profileNames)
				fmt.Printf("Enqueued %d articles × %d profiles = %d jobs\n", len(allArticles), len(profileNames), total)
			} else {
				var enqueueErr error
				if providerType != "" && modelName != "" {
					enqueueErr = client.EnqueueArticles(downlinkclient.EnqueueOptions{
						ArticleIds:   articleIds,
						ProviderType: providerType,
						ModelName:    modelName,
						FastMode:     runKeyPointsOnly,
					})
				} else {
					enqueueErr = client.EnqueueArticles(downlinkclient.EnqueueOptions{
						ArticleIds: articleIds,
						FastMode:   runKeyPointsOnly,
					})
				}
				if enqueueErr != nil {
					fmt.Printf("Failed to enqueue articles: %v\n", enqueueErr)
					return
				}
				fmt.Printf("Enqueued %d articles for analysis\n", len(allArticles))
			}

			fmt.Printf("\nUse 'downlink-cli analysis queue status' to monitor progress\n")
		},
	}

	// Add flags for analyze command
	analyzeCmd.Flags().StringVarP(&providerType, "provider", "p", "", "Override provider type")
	analyzeCmd.Flags().StringVarP(&modelName, "model", "m", "", "Override model name")
	analyzeCmd.Flags().StringSliceVar(&profileNames, "profile", nil, "LLM profile name(s) to use (comma-separated or repeated). Cannot be combined with --provider/--model.")
	analyzeCmd.Flags().BoolVar(&runSelectModel, "select-model", false, "Interactively select a model for the provider (requires --provider)")
	analyzeCmd.Flags().StringVar(&runFrom, "from", "", "Start of time window (e.g., 'now', '2025-01-01', '-7d')")
	analyzeCmd.Flags().StringVar(&runTo, "to", "", "End of time window (e.g., 'now', '2025-01-01', '-1h')")
	analyzeCmd.Flags().StringVar(&runBetween, "between", "", "Filter articles between two dates/durations (e.g., '-7d,-1d', '2025-01-01,2025-01-07')")
	analyzeCmd.Flags().BoolVar(&runDryRun, "dry-run", false, "List matching articles without analyzing")
	analyzeCmd.Flags().BoolVar(&runKeyPointsOnly, "key-points-only", false, "Extract only key points, skip other analysis")
	analyzeCmd.Flags().BoolVar(&runAllTime, "all-time", false, "Analyze articles from all time (no time window restriction)")

	// Get all analyses for an article
	getAllCmd := &cobra.Command{
		Use:     "list [article-id]",
		Aliases: []string{"ls"},
		Short:   "List all analyses for an article",
		Long:    `Retrieve all analysis results for a specific article.`,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			articleId := args[0]
			analyses, err := client.GetAllArticleAnalyses(articleId)

			if err != nil {
				fmt.Printf("Failed to get analyses: %v\n", err)
				return
			}

			if len(analyses) == 0 {
				fmt.Println("No analyses found for this article.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(analyses, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Printf("Analyses for article %s:\n\n", articleId)
				printAnalysisList(analyses)
			}
		},
	}

	// Get analysis by ID command
	getByIdCmd := &cobra.Command{
		Use:   "get [analysis-id]",
		Short: "Get analysis by ID",
		Long:  `Retrieve a specific analysis result by its ID.`,
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			analysisId := args[0]
			analysis, err := client.GetAnalysis(analysisId)

			if err != nil {
				fmt.Printf("Failed to get analysis: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(analysis, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printAnalysisDetail(analysis)
			}
		},
	}

	// Queue management commands
	queueCmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage analysis queue",
		Long:  `View and control the analysis queue.`,
	}

	// Queue status command
	queueStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show queue status",
		Long:  `Display the current state of the analysis queue.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			status := client.GetQueueStatus()

			if jsonOutput {
				out, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printQueueStatus(status)
			}
		},
	}

	// Queue start command
	queueStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Start queue processing",
		Long:  `Begin processing articles in the queue.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.StartQueue(); err != nil {
				fmt.Printf("Failed to start queue: %v\n", err)
				return
			}

			fmt.Println("Queue processing started")
		},
	}

	// Queue stop command
	queueStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop queue processing",
		Long:  `Pause processing of the queue.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.StopQueue(); err != nil {
				fmt.Printf("Failed to stop queue: %v\n", err)
				return
			}

			fmt.Println("Queue processing stopped")
		},
	}

	// Queue clear command
	queueClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear the queue",
		Long:  `Remove all articles from the queue (does not stop active processing).`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.ClearQueue(); err != nil {
				fmt.Printf("Failed to clear queue: %v\n", err)
				return
			}

			fmt.Println("Queue cleared")
		},
	}

	queueCmd.AddCommand(queueStatusCmd, queueStartCmd, queueStopCmd, queueClearCmd)

	cmd.AddCommand(getConfigCmd, updateConfigCmd, analyzeCmd, getAllCmd, getByIdCmd, queueCmd)
	return cmd
}
