package main

import (
	"fmt"
	"github.com/ma111e/downlink/cmd/server/internal/creds"
	"github.com/ma111e/downlink/cmd/server/internal/config"
	"github.com/ma111e/downlink/cmd/server/internal/feedserver"
	"github.com/ma111e/downlink/cmd/server/internal/manager"
	"github.com/ma111e/downlink/cmd/server/internal/notification"
	"github.com/ma111e/downlink/cmd/server/internal/scrapers"
	"github.com/ma111e/downlink/cmd/server/internal/store"
	"github.com/ma111e/downlink/pkg/claudeauth"
	"github.com/ma111e/downlink/pkg/codexauth"
	"github.com/ma111e/downlink/pkg/llmgateway"
	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/protos"
	"github.com/ma111e/downlink/pkg/trace"
	"github.com/ma111e/downlink/pkg/version"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/ma111e/downlink/cmd/server/internal/services"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
		PadLevelText:  true,
	})

	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)
}

func main() {
	var port int
	var host string
	var tls bool
	var certFile string
	var keyFile string
	var refresh bool
	var autoStartLightpanda bool
	var autoStartSolimen bool
	var solimenAddr string
	var feedBaseURL string
	var logLevel string
	var traceDir string
	var maxConcurrentLLMRequests int

	rootCmd := &cobra.Command{
		Use:     "server",
		Short:   "Start the DOWNLINK server",
		Version: version.String(),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Load .env file into OS env (keys not already set in environment).
			// Silently ignore missing file.
			if err := gotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
				log.WithError(err).Warn("Failed to load .env file")
			}

			// Wire all flags to DOWNLINK_* env vars via viper.
			viper.SetEnvPrefix("DOWNLINK")
			viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
			viper.AutomaticEnv()
			if err := viper.BindPFlags(cmd.Flags()); err != nil {
				return fmt.Errorf("failed to bind flags: %w", err)
			}

			// Re-populate VarP-bound variables from viper (picks up env var / .env / default).
			host = viper.GetString("host")
			port = viper.GetInt("port")
			tls = viper.GetBool("tls")
			certFile = viper.GetString("cert-file")
			keyFile = viper.GetString("key-file")
			refresh = viper.GetBool("refresh")
			autoStartLightpanda = viper.GetBool("auto-start-lightpanda")
			autoStartSolimen = viper.GetBool("auto-start-solimen")
			solimenAddr = viper.GetString("solimen-addr")
			feedBaseURL = viper.GetString("feed-base-url")
			logLevel = viper.GetString("log-level")
			traceDir = viper.GetString("trace-dir")
			maxConcurrentLLMRequests = viper.GetInt("max-concurrent-llm-requests")

			if lvl, err := log.ParseLevel(logLevel); err == nil {
				log.SetLevel(lvl)
			} else {
				log.WithField("value", logLevel).Warn("Invalid --log-level, defaulting to info")
			}

			// Content-level debug tracing: active only at the `trace` log level.
			// trace.Init resolves the dir (default /tmp/downlink-trace-<ts>) and
			// logs where content is written.
			_ = trace.Init(traceDir, log.IsLevelEnabled(log.TraceLevel))

			err := config.Init()
			if err != nil {
				log.WithError(err).Fatalln("Failed to load config")
			}

			applyGHPagesFlagOverrides(cmd)
			applyAnalysisFlagOverrides(cmd)

			initGHPages, _ := cmd.Flags().GetBool("init-gh-pages")
			reinitGHPages, _ := cmd.Flags().GetBool("reinit-gh-pages")
			if initGHPages || reinitGHPages {
				if err := runGitHubPagesInit(cmd, reinitGHPages); err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			}

			err = store.Init()
			if err != nil {
				log.WithError(err).Fatalln("Failed to initialize database")
			}

			manager.InitFeedManager(store.Db)

			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			log.Info("Starting feed aggregator")

			// Check if Lightpanda Docker container is running (needed for dynamic scraping)
			if err := scrapers.CheckLightpanda(autoStartLightpanda); err != nil {
				log.WithError(err).Warn("Lightpanda check failed; dynamic scraping may not work")
			}

			// Check if Solimen Docker container is running (needed for full_browser scraping)
			if err := scrapers.CheckSolimen(autoStartSolimen); err != nil {
				log.WithError(err).Warn("Solimen check failed; full_browser scraping may not work")
			}

			// Use the default selectors from the config
			manager.Manager.RegisterScraper("rss", scrapers.NewRSSFeedScraper(config.Config.DefaultSelectors))
			manager.Manager.RegisterScraper("html", scrapers.NewHTMLLinkScraper(config.Config.DefaultSelectors))
			// Custom scrapers can be registered here
			// manager.Manager.RegisterScraper("twitter", NewTwitterScraper())
			// manager.Manager.RegisterScraper("xcancel", scrapers.NewXcancelScraper())

			if solimenAddr != "" {
				manager.Manager.SetSolimenAddr(solimenAddr)
				log.WithField("addr", solimenAddr).Info("Solimen scraping service configured")
			} else if config.Config.SolimenAddr != "" {
				manager.Manager.SetSolimenAddr(config.Config.SolimenAddr)
				log.WithField("addr", config.Config.SolimenAddr).Info("Solimen scraping service configured")
			} else {
				log.Warn("No solimen address configured; full_browser scraping will not work")
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				os.Exit(0)
			}()

			if refresh {
				go func() {
					// Fetch all enabled feeds on startup
					var wg sync.WaitGroup
					manager.Manager.RefreshAllFeeds(&wg)
					wg.Wait()
					log.Info("Initial feed refresh completed")
				}()
			}

			// Start Atom feed server. Resolve the base URL for served feed links:
			// explicit flag/env > feed_base_url > github_pages.base_url.
			if feedBaseURL == "" {
				feedBaseURL = config.Config.FeedBaseURL
			}
			if feedBaseURL == "" {
				feedBaseURL = config.Config.Notifications.GitHubPages.BaseURL
			}
			go func() {
				atomServer := feedserver.NewFeedServer(store.Db, 65261, feedBaseURL)
				if err := atomServer.Start(); err != nil {
					log.WithError(err).Error("Atom feed server failed")
				}
			}()

			// Start server
			startServer(host, port, tls, certFile, keyFile, maxConcurrentLLMRequests)
		},
	}

	rootCmd.PersistentFlags().StringVarP(&host, "host", "H", "localhost", "gRPC server host")
	rootCmd.PersistentFlags().IntVarP(&port, "port", "p", 50051, "gRPC server port")
	rootCmd.PersistentFlags().BoolVarP(&tls, "tls", "t", false, "Enable TLS")
	rootCmd.PersistentFlags().StringVarP(&certFile, "cert-file", "c", "", "Path to server certificate")
	rootCmd.PersistentFlags().StringVarP(&keyFile, "key-file", "k", "", "Path to server key")
	rootCmd.PersistentFlags().BoolVarP(&refresh, "refresh", "r", false, "Refresh all feeds on startup")
	rootCmd.PersistentFlags().BoolVar(&autoStartLightpanda, "auto-start-lightpanda", false, "Automatically start the Lightpanda Docker container if not running (skip interactive prompt)")
	rootCmd.PersistentFlags().BoolVar(&autoStartSolimen, "auto-start-solimen", false, "Automatically start the Solimen Docker container if not running (skip interactive prompt)")
	rootCmd.PersistentFlags().StringVar(&solimenAddr, "solimen-addr", "http://localhost:5011", "Solimen service address for full_browser scraping (e.g. http://localhost:5011)")
	rootCmd.PersistentFlags().StringVar(&feedBaseURL, "feed-base-url", "", "Base URL for served Atom feed links (e.g. https://feeds.example.com)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (trace, debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&traceDir, "trace-dir", "", "Directory for content-level debug traces (LLM prompt/response, raw feed/scrape bodies); only active at --log-level trace. Default: /tmp/downlink-trace-<timestamp>")
	rootCmd.PersistentFlags().IntVar(&maxConcurrentLLMRequests, "max-concurrent-llm-requests", 1, "Maximum number of concurrent LLM analysis requests (default: 1)")
	rootCmd.PersistentFlags().Bool("auto-analyze", false, "Automatically enqueue articles for analysis after each feed refresh [overrides config]")
	rootCmd.PersistentFlags().Bool("vibe-score", false, "Use the legacy single-number LLM importance prompt instead of the rubric scoring system [overrides config]")
	rootCmd.PersistentFlags().Bool("glossary", false, "Generate glossary-mode content (plain-language explanation + jargon glossary) per article [overrides config]")
	//rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./excuses-client.yml)")

	rootCmd.PersistentFlags().Bool("gh-pages-enabled", false, "Enable GitHub Pages publishing [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-repo", "", "GitHub Pages repo URL, e.g. https://github.com/user/repo.git [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-branch", "", "Branch to push to (default 'main') [overrides config]")
	rootCmd.PersistentFlags().Bool("gh-pages-configure", false, "Configure GitHub Pages source to --gh-pages-branch at '/' before publishing [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-token", "", "GitHub PAT with contents:write (prefer DOWNLINK_GH_PAGES_TOKEN env) [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-output-dir", "", "Subdirectory inside the repo for digest files [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-base-url", "", "Public base URL of the Pages site, e.g. https://user.github.io [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-commit-author", "", "Commit author name [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-commit-email", "", "Commit author email [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-clone-dir", "", "Local working clone directory [overrides config]")
	rootCmd.PersistentFlags().String("gh-pages-discord-webhook", "", "Discord webhook URL to notify when a page is published [overrides config]")

	rootCmd.PersistentFlags().Bool("init-gh-pages", false, "Initialize the GitHub Pages repository and exit (idempotent, existing files are not overwritten; use --reinit-gh-pages to wipe first)")
	rootCmd.PersistentFlags().Bool("reinit-gh-pages", false, "Erase and reinitialize the GitHub Pages repository from scratch (destructive, prompts for confirmation)")

	rootCmd.AddCommand(newDevCommand())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// runGitHubPagesInit runs --init-gh-pages or --reinit-gh-pages and exits.
// It is called from PersistentPreRunE after config and flag overrides are applied
// but before database initialisation.
func runGitHubPagesInit(_ *cobra.Command, reinit bool) error {
	ghCfg := config.Config.Notifications.GitHubPages
	if ghCfg.Token == "" {
		ghCfg.Token = os.Getenv("DOWNLINK_GH_PAGES_TOKEN")
	}
	if ghCfg.RepoURL == "" {
		return fmt.Errorf("--gh-pages-repo is required")
	}
	if ghCfg.Token == "" {
		return fmt.Errorf("GitHub Pages token required (--gh-pages-token or DOWNLINK_GH_PAGES_TOKEN)")
	}

	if reinit {
		fmt.Fprintln(os.Stderr, "WARNING: --reinit-gh-pages will DELETE ALL content from the remote")
		fmt.Fprintf(os.Stderr, "branch %q and start fresh. Type \"yes\" to confirm: ", ghCfg.Branch)
		var answer string
		fmt.Fscan(os.Stdin, &answer)
		if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
			fmt.Fprintln(os.Stderr, "Aborted.")
			os.Exit(1)
		}
	}

	publisher := notification.NewGitHubPagesPublisher(ghCfg)
	return publisher.InitPages(reinit)
}

// applyAnalysisFlagOverrides copies CLI flag / env var values into the analysis config.
func applyAnalysisFlagOverrides(cmd *cobra.Command) {
	a := &config.Config.Analysis
	if cmd.Flags().Changed("auto-analyze") {
		v, _ := cmd.Flags().GetBool("auto-analyze")
		a.AutoAnalyze = v
	} else if viper.IsSet("auto-analyze") {
		a.AutoAnalyze = viper.GetBool("auto-analyze")
	}
	if cmd.Flags().Changed("vibe-score") {
		v, _ := cmd.Flags().GetBool("vibe-score")
		a.VibeScore = v
	} else if viper.IsSet("vibe-score") {
		a.VibeScore = viper.GetBool("vibe-score")
	}
	if cmd.Flags().Changed("glossary") {
		v, _ := cmd.Flags().GetBool("glossary")
		a.Glossary = v
	} else if viper.IsSet("glossary") {
		a.Glossary = viper.GetBool("glossary")
	}
}

// applyGHPagesFlagOverrides copies CLI flag / env var values into the loaded config.
// CLI flags take highest priority; env vars (including .env) override config file values.
func applyGHPagesFlagOverrides(cmd *cobra.Command) {
	gh := &config.Config.Notifications.GitHubPages
	if cmd.Flags().Changed("gh-pages-enabled") {
		v, _ := cmd.Flags().GetBool("gh-pages-enabled")
		gh.Enabled = v
	} else if viper.IsSet("gh-pages-enabled") {
		gh.Enabled = viper.GetBool("gh-pages-enabled")
	}
	if cmd.Flags().Changed("gh-pages-repo") {
		gh.RepoURL, _ = cmd.Flags().GetString("gh-pages-repo")
	} else if viper.IsSet("gh-pages-repo") {
		gh.RepoURL = viper.GetString("gh-pages-repo")
	}
	if cmd.Flags().Changed("gh-pages-branch") {
		gh.Branch, _ = cmd.Flags().GetString("gh-pages-branch")
	} else if viper.IsSet("gh-pages-branch") {
		gh.Branch = viper.GetString("gh-pages-branch")
	}
	if cmd.Flags().Changed("gh-pages-configure") {
		v, _ := cmd.Flags().GetBool("gh-pages-configure")
		gh.ConfigurePages = v
	} else if viper.IsSet("gh-pages-configure") {
		gh.ConfigurePages = viper.GetBool("gh-pages-configure")
	}
	if cmd.Flags().Changed("gh-pages-token") {
		gh.Token, _ = cmd.Flags().GetString("gh-pages-token")
	} else if viper.IsSet("gh-pages-token") {
		gh.Token = viper.GetString("gh-pages-token")
	}
	if cmd.Flags().Changed("gh-pages-output-dir") {
		gh.OutputDir, _ = cmd.Flags().GetString("gh-pages-output-dir")
	} else if viper.IsSet("gh-pages-output-dir") {
		gh.OutputDir = viper.GetString("gh-pages-output-dir")
	}
	if cmd.Flags().Changed("gh-pages-base-url") {
		gh.BaseURL, _ = cmd.Flags().GetString("gh-pages-base-url")
	} else if viper.IsSet("gh-pages-base-url") {
		gh.BaseURL = viper.GetString("gh-pages-base-url")
	}
	if cmd.Flags().Changed("gh-pages-commit-author") {
		gh.CommitAuthor, _ = cmd.Flags().GetString("gh-pages-commit-author")
	} else if viper.IsSet("gh-pages-commit-author") {
		gh.CommitAuthor = viper.GetString("gh-pages-commit-author")
	}
	if cmd.Flags().Changed("gh-pages-commit-email") {
		gh.CommitEmail, _ = cmd.Flags().GetString("gh-pages-commit-email")
	} else if viper.IsSet("gh-pages-commit-email") {
		gh.CommitEmail = viper.GetString("gh-pages-commit-email")
	}
	if cmd.Flags().Changed("gh-pages-clone-dir") {
		gh.CloneDir, _ = cmd.Flags().GetString("gh-pages-clone-dir")
	} else if viper.IsSet("gh-pages-clone-dir") {
		gh.CloneDir = viper.GetString("gh-pages-clone-dir")
	}
	if cmd.Flags().Changed("gh-pages-discord-webhook") {
		gh.DiscordWebhookURL, _ = cmd.Flags().GetString("gh-pages-discord-webhook")
	} else if viper.IsSet("gh-pages-discord-webhook") {
		gh.DiscordWebhookURL = viper.GetString("gh-pages-discord-webhook")
	}
}

func startServer(host string, port int, tls bool, certFile, keyFile string, maxConcurrentLLMRequests int) {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	var opts []grpc.ServerOption
	if tls {
		if certFile == "" {
			log.Fatalln("--cert-file is required when --tls is enabled")
		}
		if keyFile == "" {
			log.Fatalln("--key-file is required when --tls is enabled")
		}

		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			log.Fatalf("Failed to generate credentials: %v", err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	grpcServer := grpc.NewServer(opts...)

	// Single LLM gateway shared by every call path: direct analysis,
	// queued analysis, digest dedupe/summary, digest article re-analysis.
	// This is the only place --max-concurrent-llm-requests is actually enforced.
	gw := llmgateway.New(maxConcurrentLLMRequests)

	// Codex OAuth manager: wires the credential pool to config.json persistence.
	config.CodexManager = codexauth.NewManager(
		func() *models.ServerConfig { return config.Config },
		config.SaveConfig,
	)

	// Claude Code OAuth manager: same wiring for claude-code subscription auth.
	config.ClaudeManager = claudeauth.NewManager(
		func() *models.ServerConfig { return config.Config },
		config.SaveConfig,
	)

	llmsServer := services.NewLLMsServer(gw)
	digestServer := services.NewDigestServer(gw, llmsServer)

	protos.RegisterArticleServiceServer(grpcServer, services.NewArticleServer())
	protos.RegisterAnalysisServiceServer(grpcServer, services.NewAnalysisServer())
	protos.RegisterCategoriesServiceServer(grpcServer, services.NewCategoriesServer())
	queueServer := services.NewQueueServer(llmsServer, maxConcurrentLLMRequests)
	protos.RegisterFeedsServiceServer(grpcServer, services.NewFeedsServer(queueServer, gw))
	protos.RegisterDigestServiceServer(grpcServer, digestServer)
	protos.RegisterLLMsServiceServer(grpcServer, llmsServer)
	protos.RegisterQueueServiceServer(grpcServer, queueServer)
	protos.RegisterServerConfigServiceServer(grpcServer, services.NewServerConfigServer())
	protos.RegisterCredsServiceServer(grpcServer, creds.NewService(config.CodexManager, config.ClaudeManager))

	log.WithFields(log.Fields{
		"host": host,
		"port": port,
		"tls":  tls,
	}).Info("gRPC server started")
	log.WithField("max_concurrent_llm_requests", maxConcurrentLLMRequests).Info("LLM gateway concurrency configured")
	grpcServer.Serve(lis)
}
