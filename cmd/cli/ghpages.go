package main

import (
	"bufio"
	"downlink/cmd/server/notification"
	"downlink/pkg/models"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func createGHPagesCommands() *cobra.Command {
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
		Use:   "ghpages",
		Short: "Manage the GitHub Pages publishing repository",
		Long: `Set up or reset the GitHub Pages repository that downlink publishes digests to.

These commands do not require a running downlink server — they connect to
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
Run again safely — if nothing has changed it exits without committing.`,
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
			fmt.Fprintln(os.Stderr, "WARNING: ghpages reinit will DELETE ALL content from the remote")
			fmt.Fprintf(os.Stderr, "branch %q and start fresh. Type \"yes\" to confirm: ", branchName)
			var answer string
			fmt.Fscan(os.Stdin, &answer)
			if strings.TrimSpace(strings.ToLower(answer)) != "yes" {
				fmt.Fprintln(os.Stderr, "Aborted.")
				os.Exit(1)
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
			return publisher.InitPages(true)
		},
	}

	digestCmd := &cobra.Command{
		Use:   "digest",
		Short: "Add or remove a digest from the GitHub Pages archive",
	}

	addCmd := &cobra.Command{
		Use:   "add <digest-id>",
		Short: "Fetch a digest from the server and publish it to the GitHub Pages archive",
		Long: `Fetch the digest from the running downlink server, render it to HTML,
add it to the archive manifest, and push the result to GitHub Pages.

This command requires a running downlink server (--address / --port).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := buildConfig()
			if err != nil {
				return err
			}
			client := getNewDownlinkClient()
			digest, err := client.GetDigest(args[0])
			if err != nil {
				return fmt.Errorf("fetch digest: %w", err)
			}
			publisher := notification.NewGitHubPagesPublisher(cfg)
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
				title, err = selectFromList(titles)
				if err != nil {
					return err
				}
			}

			return publisher.RemoveDigest(title)
		},
	}

	digestCmd.AddCommand(addCmd, removeCmd)
	cmd.AddCommand(initCmd, reinitCmd, digestCmd)
	return cmd
}

// selectFromList prints a numbered list of items to stderr and reads the
// user's choice from stdin. Returns the selected item.
func selectFromList(items []string) (string, error) {
	fmt.Fprintln(os.Stderr, "Select a digest to remove:")
	for i, item := range items {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, item)
	}
	fmt.Fprintf(os.Stderr, "Enter number (1-%d): ", len(items))

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input provided")
	}
	raw := strings.TrimSpace(scanner.Text())
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > len(items) {
		return "", fmt.Errorf("invalid selection %q: enter a number between 1 and %d", raw, len(items))
	}
	return items[n-1], nil
}
