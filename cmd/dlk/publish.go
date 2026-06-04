package main

import (
	"fmt"
	"github.com/ma111e/downlink/cmd/server/notification"
	"github.com/ma111e/downlink/pkg/models"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func createPublishCommands() *cobra.Command {
	var (
		repo           string
		branch         string
		token          string
		outputDir      string
		configurePages bool
		cloneDir       string
		commitAuthor   string
		commitEmail    string
	)

	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Manage digest publishing to GitHub Pages",
		Long: `Set up or reset the GitHub Pages repository that downlink publishes digests to.

These commands do not require a running downlink server; they connect to
GitHub directly using the provided token.`,
	}

	cmd.PersistentFlags().StringVar(&repo, "repo", "", "GitHub Pages repo HTTPS URL, e.g. https://github.com/user/user.github.io.git (required)")
	cmd.PersistentFlags().StringVar(&branch, "branch", "", "Branch to publish to (default: main)")
	cmd.PersistentFlags().StringVar(&token, "token", "", "GitHub PAT with contents:write (prefer DOWNLINK_GH_PAGES_TOKEN env var)")
	cmd.PersistentFlags().StringVar(&outputDir, "output-dir", "", "Subdirectory inside the repo for digest files (default: digests)")
	cmd.PersistentFlags().BoolVar(&configurePages, "configure-pages", false, "Configure the GitHub Pages source to --branch at / via the API (requires Pages+Administration token permissions)")
	cmd.PersistentFlags().StringVar(&cloneDir, "clone-dir", "", "Local directory for the repo clone (default: $TMPDIR/downlink-ghpages)")
	cmd.PersistentFlags().StringVar(&commitAuthor, "commit-author", "", "Git commit author name (default: downlink-bot)")
	cmd.PersistentFlags().StringVar(&commitEmail, "commit-email", "", "Git commit author email (default: downlink-bot@users.noreply.github.com)")

	buildConfig := func() (models.GitHubPagesNotificationConfig, error) {
		envBool := func(key string, current bool) (bool, error) {
			raw := strings.TrimSpace(os.Getenv(key))
			if raw == "" {
				return current, nil
			}
			parsed, err := strconv.ParseBool(raw)
			if err != nil {
				return false, fmt.Errorf("invalid %s value %q: %w", key, raw, err)
			}
			return parsed, nil
		}
		envString := func(key, current string) string {
			if current != "" {
				return current
			}
			return os.Getenv(key)
		}

		repo = envString("DOWNLINK_GH_PAGES_REPO", repo)
		branch = envString("DOWNLINK_GH_PAGES_BRANCH", branch)
		outputDir = envString("DOWNLINK_GH_PAGES_OUTPUT_DIR", outputDir)
		cloneDir = envString("DOWNLINK_GH_PAGES_CLONE_DIR", cloneDir)
		commitAuthor = envString("DOWNLINK_GH_PAGES_COMMIT_AUTHOR", commitAuthor)
		commitEmail = envString("DOWNLINK_GH_PAGES_COMMIT_EMAIL", commitEmail)
		var err error
		configurePages, err = envBool("DOWNLINK_GH_PAGES_CONFIGURE", configurePages)
		if err != nil {
			return models.GitHubPagesNotificationConfig{}, err
		}

		tok := token
		if tok == "" {
			tok = os.Getenv("DOWNLINK_GH_PAGES_TOKEN")
		}
		if repo == "" {
			return models.GitHubPagesNotificationConfig{}, fmt.Errorf("--repo is required")
		}
		if tok == "" {
			return models.GitHubPagesNotificationConfig{}, fmt.Errorf("token required: use --token or set DOWNLINK_GH_PAGES_TOKEN")
		}
		return models.GitHubPagesNotificationConfig{
			RepoURL:        repo,
			Branch:         branch,
			Token:          tok,
			OutputDir:      outputDir,
			ConfigurePages: configurePages,
			CloneDir:       cloneDir,
			CommitAuthor:   commitAuthor,
			CommitEmail:    commitEmail,
		}, nil
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the GitHub Pages repository (idempotent)",
		Long: `Create the remote branch if absent, seed an initial manifest.json and
index pages, then commit and push. Existing files are not overwritten.
Run again safely; if nothing has changed it exits without committing.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
			return publisher.InitPages(false)
		},
	}

	reinitCmd := &cobra.Command{
		Use:   "reinit",
		Short: "Erase and reinitialize the GitHub Pages repository (destructive)",
		Long: `Delete the remote branch and the local clone, then recreate both from
scratch with a fresh manifest and index. All existing digest HTML files on
the branch will be lost. Prompts for confirmation before proceeding.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			branchName := cfg.Branch
			if branchName == "" {
				branchName = "main"
			}
			confirm := false
			flushStdin()
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Delete ALL content from branch %q and reinitialise?", branchName)).
				Description("This will erase all existing digest HTML files on the remote branch.").
				Affirmative("Yes, reinitialise").
				Negative("No, abort").
				Value(&confirm).
				Run(); err != nil || !confirm {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return nil
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
			return publisher.InitPages(true)
		},
	}

	addCmd := &cobra.Command{
		Use:   "add [digest-id]",
		Short: "Fetch a digest from the server and publish it to the GitHub Pages archive",
		Long: `Fetch the digest from the running downlink server, render it to HTML,
add it to the archive manifest, and push the result to GitHub Pages.

When no digest ID is provided an interactive list of available digests is shown.

This command requires a running downlink server (--address / --port).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()

			digestID := ""
			if len(args) == 1 {
				digestID = args[0]
			} else {
				digests, err := client.ListDigests(0)
				if err != nil {
					return fmt.Errorf("list digests: %w", err)
				}
				if len(digests) == 0 {
					return fmt.Errorf("no digests found on the server")
				}
				options := make([]huh.Option[string], len(digests))
				for i, d := range digests {
					title := d.Title
					if title == "" {
						title = "(untitled)"
					}
					articleCount := 0
					if d.ArticleCount != nil {
						articleCount = *d.ArticleCount
					}
					label := fmt.Sprintf("%s  %s  (%d articles)",
						d.CreatedAt.Format("2006-01-02 15:04"), title, articleCount)
					options[i] = huh.NewOption(label, d.Id)
				}
				if err := huh.NewSelect[string]().
					Title("Select a digest to publish").
					Options(options...).
					Value(&digestID).
					Run(); err != nil {
					return err
				}
			}

			digest, err := client.GetDigest(digestID)
			if err != nil {
				return fmt.Errorf("fetch digest: %w", err)
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
			publisher.SetDigestLister(func(n int) ([]models.Digest, error) {
				return client.ListDigestsFull(n)
			})
			return publisher.SendDigest(digest)
		},
	}

	removeCmd := &cobra.Command{
		Use:   "remove [title]",
		Short: "Remove a digest from the GitHub Pages archive and republish",
		Long: `Look up the digest by title in the archive manifest, remove its digest
and swipe HTML files, update the manifest, and push the result to GitHub Pages.

The title is matched case-insensitively against manifest entries.
When no title is given, an interactive list is shown to pick from.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)

			title := ""
			if len(args) == 1 {
				title = args[0]
			} else {
				titles, err := publisher.ManifestTitles()
				if err != nil {
					return err
				}
				if len(titles) == 0 {
					return fmt.Errorf("no digests found in the manifest")
				}
				options := make([]huh.Option[string], len(titles))
				for i, t := range titles {
					options[i] = huh.NewOption(t, t)
				}
				if err := huh.NewSelect[string]().
					Title("Select a digest to remove").
					Options(options...).
					Value(&title).
					Run(); err != nil {
					return err
				}
			}

			_, err = publisher.RemoveDigest(title)
			return err
		},
	}

	var republishTheme string
	var republishDryRun bool
	var republishNoWait bool

	republishAllCmd := &cobra.Command{
		Use:   "republish-all",
		Short: "Re-render all published digests with the current templates",
		Long: `Fetch every digest from the running downlink server, filter to those already
present in the GitHub Pages manifest, re-render each page with the current
templates, rebuild the manifest, and push the result as a single commit.

Use --dry-run to render and stage locally without committing or pushing.

This command requires a running downlink server (--address / --port).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()

			summaries, err := client.ListDigests(0)
			if err != nil {
				return fmt.Errorf("list digests: %w", err)
			}
			if len(summaries) == 0 {
				fmt.Fprintln(os.Stderr, "No digests found on the server.")
				return nil
			}

			publisher := notification.NewGitHubPagesPublisher(cfg)
			return runPublishWithProgress(publisher, func(prog notification.PublishProgress) error {
				prog.Start("fetch", fmt.Sprintf("Fetching %d digests", len(summaries)))
				digests := make([]models.Digest, 0, len(summaries))
				for _, s := range summaries {
					d, err := client.GetDigest(s.Id)
					if err != nil {
						prog.Complete("fetch", false, "fetch failed")
						return fmt.Errorf("fetch digest %s: %w", s.Id, err)
					}
					digests = append(digests, d)
				}
				prog.Complete("fetch", true, fmt.Sprintf("fetched %d digests", len(digests)))
				return publisher.RepublishAll(digests, republishTheme, republishDryRun, !republishNoWait)
			})
		},
	}
	republishAllCmd.Flags().StringVar(&republishTheme, "theme", "dark", "Theme to use when re-rendering digest pages")
	republishAllCmd.Flags().BoolVar(&republishDryRun, "dry-run", false, "Render and stage locally without committing or pushing")
	republishAllCmd.Flags().BoolVar(&republishNoWait, "no-wait", false, "Push and exit without waiting for the GitHub Pages deploy")

	var republishIndexDryRun bool

	republishIndexCmd := &cobra.Command{
		Use:   "republish-index",
		Short: "Re-render the archive index pages with the current templates",
		Long: `Re-render the archive index pages (both the digest-subdirectory index and
the root index.html) with the current templates and push the result as a
single commit. Digest HTML files and the manifest are not modified.

Use --dry-run to write the files locally without committing or pushing.

This command does not require a running downlink server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
			return runPublishWithProgress(publisher, func(prog notification.PublishProgress) error {
				return publisher.RepublishIndex(republishIndexDryRun, !republishNoWait)
			})
		},
	}
	republishIndexCmd.Flags().BoolVar(&republishIndexDryRun, "dry-run", false, "Write index files locally without committing or pushing")
	republishIndexCmd.Flags().BoolVar(&republishNoWait, "no-wait", false, "Push and exit without waiting for the GitHub Pages deploy")

	republishCmd := &cobra.Command{
		Use:   "republish [digest-id-or-title]",
		Short: "Remove and re-publish a single digest with the current templates",
		Long: `Remove the digest from the GitHub Pages archive and re-publish it with the
current templates. Equivalent to running remove followed by add.

Accepts a digest ID or full title as argument. When no argument is given,
an interactive list of available digests is shown.

This command requires a running downlink server (--address / --port).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()
			publisher := notification.NewGitHubPagesPublisher(cfg)
			publisher.SetDigestLister(func(n int) ([]models.Digest, error) {
				return client.ListDigestsFull(n)
			})

			var digest models.Digest
			if len(args) == 1 {
				digest, err = client.GetDigest(args[0])
				if err != nil {
					all, listErr := client.ListDigests(0)
					if listErr != nil {
						return fmt.Errorf("fetch digest: %w", err)
					}
					found := false
					for _, d := range all {
						if strings.EqualFold(d.Title, args[0]) {
							digest, err = client.GetDigest(d.Id)
							if err != nil {
								return fmt.Errorf("fetch digest: %w", err)
							}
							found = true
							break
						}
					}
					if !found {
						return fmt.Errorf("digest not found: %q", args[0])
					}
				}
			} else {
				digests, err := client.ListDigests(0)
				if err != nil {
					return fmt.Errorf("list digests: %w", err)
				}
				if len(digests) == 0 {
					return fmt.Errorf("no digests found on the server")
				}
				options := make([]huh.Option[string], len(digests))
				for i, d := range digests {
					title := d.Title
					if title == "" {
						title = "(untitled)"
					}
					articleCount := 0
					if d.ArticleCount != nil {
						articleCount = *d.ArticleCount
					}
					label := fmt.Sprintf("%s  %s  (%d articles)",
						d.CreatedAt.Format("2006-01-02 15:04"), title, articleCount)
					options[i] = huh.NewOption(label, d.Id)
				}
				var digestID string
				if err := huh.NewSelect[string]().
					Title("Select a digest to republish").
					Options(options...).
					Value(&digestID).
					Run(); err != nil {
					return err
				}
				digest, err = client.GetDigest(digestID)
				if err != nil {
					return fmt.Errorf("fetch digest: %w", err)
				}
			}

			return runPublishWithProgress(publisher, func(prog notification.PublishProgress) error {
				return publisher.Republish(digest, !republishNoWait)
			})
		},
	}
	republishCmd.Flags().BoolVar(&republishNoWait, "no-wait", false, "Push and exit without waiting for the GitHub Pages deploy")

	cmd.AddCommand(initCmd, reinitCmd, addCmd, removeCmd, republishAllCmd, republishIndexCmd, republishCmd)
	return cmd
}
