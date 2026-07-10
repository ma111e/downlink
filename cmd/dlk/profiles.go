package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// createProfileCommands builds the `profiles` command group: the runtime
// counterpart of the startup profiles.yml apply, mirroring `feeds apply`.
func createProfileCommands() *cobra.Command {
	profilesCmd := &cobra.Command{
		Use:     "profiles",
		Aliases: []string{"profile"},
		Short:   "Manage editorial profiles",
		Long:    `Manage editorial profiles (the profiles.yml catalog): reconcile from a file or list the stored profiles.`,
	}

	var applyFile string
	var applyDryRun bool
	applyCmd := &cobra.Command{
		Use:   "apply",
		Short: "Reconcile profiles from a file",
		Long: `Reconcile the database to match a profiles YAML file: profiles in the file are
created or updated (with their feed pools re-resolved), and profiles no longer
present are disabled (their digests and analyses are kept; the default profile
is never touched). Re-adding a disabled profile to the file re-enables it.`,
		Run: func(cmd *cobra.Command, args []string) {
			data, err := os.ReadFile(applyFile)
			if err != nil {
				fmt.Printf("%s %v\n", styleErr.Render("✗"), err)
				return
			}

			client := getNewDownlinkClient()
			res, err := client.ApplyProfiles(data, applyDryRun)
			if err != nil {
				fmt.Printf("Failed to apply profiles: %v\n", err)
				return
			}

			if jsonOutput {
				out, _ := json.MarshalIndent(map[string][]string{
					"created": res.Created, "updated": res.Updated, "disabled": res.Disabled,
					"skipped": res.Skipped, "warnings": res.Warnings,
				}, "", "  ")
				fmt.Println(string(out))
				return
			}

			if applyDryRun {
				fmt.Println(styleWarn.Render("DRY RUN: no changes applied"))
			}
			for _, w := range res.Warnings {
				fmt.Printf("%s %s\n", styleWarn.Render("!"), w)
			}
			printFeedActionList("Created", res.Created)
			printFeedActionList("Updated", res.Updated)
			printFeedActionList("Disabled", res.Disabled)
			printFeedActionList("Skipped", res.Skipped)
			if len(res.Created)+len(res.Updated)+len(res.Disabled)+len(res.Skipped) == 0 {
				fmt.Println(styleDim.Render("Nothing to do; no profiles in the file."))
			}
		},
	}
	applyCmd.Flags().StringVarP(&applyFile, "file", "f", "", "Path to profiles YAML file (required)")
	applyCmd.Flags().BoolVar(&applyDryRun, "dry-run", false, "Show what would change without applying")
	_ = applyCmd.MarkFlagRequired("file")

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List stored profiles",
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()
			res, err := client.ListProfiles()
			if err != nil {
				fmt.Printf("Failed to list profiles: %v\n", err)
				return
			}

			if jsonOutput {
				out, _ := json.MarshalIndent(res.Profiles, "", "  ")
				fmt.Println(string(out))
				return
			}

			tw := newTable("SLUG", "NAME", "ENABLED", "FEEDS", "SUBDIR", "LAYOUT", "THEME")
			for _, p := range res.Profiles {
				subdir := p.OutputSubdir
				if subdir == "" {
					subdir = dash
				}
				layout := p.Layout
				if layout == "" {
					layout = dash
				}
				theme := p.Theme
				if theme == "" {
					theme = dash
				}
				fmt.Fprintf(tw, "%s\t%s\t%v\t%d\t%s\t%s\t%s\n",
					p.Slug, truncate(p.Name, 30), p.Enabled, p.FeedCount, subdir, layout, theme)
			}
			_ = tw.Flush()
		},
	}

	profilesCmd.AddCommand(applyCmd)
	profilesCmd.AddCommand(listCmd)
	return profilesCmd
}
