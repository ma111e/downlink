package main

import (
	"downlink/cmd/server/internal/config"
	"downlink/cmd/server/internal/feedserver"
	"downlink/cmd/server/internal/manager"
	"downlink/cmd/server/internal/scrapers"
	"downlink/cmd/server/internal/store"
	"downlink/pkg/llmgateway"
	"downlink/pkg/protos"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"downlink/cmd/server/internal/services"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
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
	var logLevel string
	var maxConcurrentLLMRequests int

	rootCmd := &cobra.Command{
		Use:   "server",
		Short: "Start the DOWNLINK server",
		PreRun: func(cmd *cobra.Command, args []string) {
			if lvl, err := log.ParseLevel(logLevel); err == nil {
				log.SetLevel(lvl)
			} else {
				log.WithField("value", logLevel).Warn("Invalid --log-level, defaulting to info")
			}

			err := config.Init()
			if err != nil {
				log.WithError(err).Fatalln("Failed to load config")
			}

			applyGHPagesFlagOverrides(cmd)

			err = store.Init()
			if err != nil {
				log.WithError(err).Fatalln("Failed to initialize database")
			}

			manager.InitFeedManager(store.Db)

			// Initialize worker pool for analyzes
			// workerpool.InitPool()
			// log.WithField("max_workers", config.Config.Analysis.WorkerPool.MaxWorkers).Info("Analysis worker pool initialized")
		},
		Run: func(cmd *cobra.Command, args []string) {
			log.Info("Starting feed aggregator")

			// Check if Lightpanda Docker container is running (needed for dynamic scraping)
			if err := scrapers.CheckLightpanda(autoStartLightpanda); err != nil {
				log.WithError(err).Warn("Lightpanda check failed — dynamic scraping may not work")
			}

			// Check if Solimen Docker container is running (needed for full_browser scraping)
			if err := scrapers.CheckSolimen(autoStartSolimen); err != nil {
				log.WithError(err).Warn("Solimen check failed — full_browser scraping may not work")
			}

			// Use the default selectors from the config
			manager.Manager.RegisterScraper("rss", scrapers.NewRSSFeedScraper(config.Config.DefaultSelectors))
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
				log.Warn("No solimen address configured — full_browser scraping will not work")
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				os.Exit(0)
			}()

			if err := registerConfiguredFeeds(); err != nil {
				log.WithError(err).Fatalln("Failed to register configured feeds")
			}

			// Disable any feeds in the database that are not found in the configuration file
			dbFeeds, err := store.Db.ListFeeds()
			if err != nil {
				log.Errorf("failed to list feeds from database: %v", err)
			}
			configFeedURLs := make(map[string]struct{}, len(config.Config.Feeds))
			for _, configFeed := range config.Config.Feeds {
				configFeedURLs[configFeed.URL] = struct{}{}
			}
			for _, dbFeed := range dbFeeds {
				_, found := configFeedURLs[dbFeed.URL]

				if !found || dbFeed.Enabled == nil || !*dbFeed.Enabled {
					err = manager.Manager.UpdateFeedEnabled(dbFeed.Id, false)
					if err != nil {
						log.Errorf("failed to update feed in database: %v", err)
					}
				}
			}

			if refresh {
				go func() {
					// Fetch all enabled feeds on startup
					var wg sync.WaitGroup
					manager.Manager.RefreshAllFeeds(&wg)
					wg.Wait()
					log.Info("Initial feed refresh completed")
				}()
			}

			// Start Atom feed server
			go func() {
				atomServer := feedserver.NewFeedServer(store.Db, 65261)
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
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level (trace, debug, info, warn, error)")
	rootCmd.PersistentFlags().IntVar(&maxConcurrentLLMRequests, "max-concurrent-llm-requests", 1, "Maximum number of concurrent LLM analysis requests (default: 1)")
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

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// applyGHPagesFlagOverrides copies explicitly-set CLI flag values into the loaded config.
// Only flags that the user actually provided override the config file values.
func applyGHPagesFlagOverrides(cmd *cobra.Command) {
	gh := &config.Config.Notifications.GitHubPages
	if cmd.Flags().Changed("gh-pages-enabled") {
		v, _ := cmd.Flags().GetBool("gh-pages-enabled")
		gh.Enabled = v
	}
	if cmd.Flags().Changed("gh-pages-repo") {
		gh.RepoURL, _ = cmd.Flags().GetString("gh-pages-repo")
	}
	if cmd.Flags().Changed("gh-pages-branch") {
		gh.Branch, _ = cmd.Flags().GetString("gh-pages-branch")
	}
	if cmd.Flags().Changed("gh-pages-configure") {
		v, _ := cmd.Flags().GetBool("gh-pages-configure")
		gh.ConfigurePages = v
	}
	if cmd.Flags().Changed("gh-pages-token") {
		gh.Token, _ = cmd.Flags().GetString("gh-pages-token")
	}
	if cmd.Flags().Changed("gh-pages-output-dir") {
		gh.OutputDir, _ = cmd.Flags().GetString("gh-pages-output-dir")
	}
	if cmd.Flags().Changed("gh-pages-base-url") {
		gh.BaseURL, _ = cmd.Flags().GetString("gh-pages-base-url")
	}
	if cmd.Flags().Changed("gh-pages-commit-author") {
		gh.CommitAuthor, _ = cmd.Flags().GetString("gh-pages-commit-author")
	}
	if cmd.Flags().Changed("gh-pages-commit-email") {
		gh.CommitEmail, _ = cmd.Flags().GetString("gh-pages-commit-email")
	}
	if cmd.Flags().Changed("gh-pages-clone-dir") {
		gh.CloneDir, _ = cmd.Flags().GetString("gh-pages-clone-dir")
	}
	if cmd.Flags().Changed("gh-pages-discord-webhook") {
		gh.DiscordWebhookURL, _ = cmd.Flags().GetString("gh-pages-discord-webhook")
	}
}

// registerConfiguredFeeds registers all feeds from the configuration
func registerConfiguredFeeds() error {
	var errs []error
	for _, feedConfig := range config.Config.Feeds {

		if err := manager.Manager.RegisterFeed(feedConfig); err != nil {
			log.WithFields(log.Fields{
				"url":  feedConfig.URL,
				"type": feedConfig.Type,
				"err":  err,
			}).Error("Failed to register feed")
			errs = append(errs, fmt.Errorf("%s (%s): %w", feedConfig.URL, feedConfig.Type, err))
		}
	}
	return errors.Join(errs...)
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

	llmsServer := services.NewLLMsServer(gw)
	digestServer := services.NewDigestServer(gw, llmsServer)

	protos.RegisterArticleServiceServer(grpcServer, services.NewArticleServer())
	protos.RegisterAnalysisServiceServer(grpcServer, services.NewAnalysisServer())
	protos.RegisterCategoriesServiceServer(grpcServer, services.NewCategoriesServer())
	protos.RegisterFeedsServiceServer(grpcServer, services.NewFeedsServer())
	protos.RegisterDigestServiceServer(grpcServer, digestServer)
	protos.RegisterLLMsServiceServer(grpcServer, llmsServer)
	protos.RegisterQueueServiceServer(grpcServer, services.NewQueueServer(llmsServer, maxConcurrentLLMRequests))
	protos.RegisterServerConfigServiceServer(grpcServer, services.NewServerConfigServer())

	log.WithFields(log.Fields{
		"host": host,
		"port": port,
		"tls":  tls,
	}).Info("gRPC server started")
	log.WithField("max_concurrent_llm_requests", maxConcurrentLLMRequests).Info("LLM gateway concurrency configured")
	grpcServer.Serve(lis)
}
