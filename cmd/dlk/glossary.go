package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func createGlossaryCommands() *cobra.Command {
	glossaryCmd := &cobra.Command{
		Use:   "glossary",
		Short: "Inspect and curate the global glossary",
		Long: `List glossary entries and set curated definitions.

The glossary is built automatically during digest generation (with --glossary). A curated
definition set via 'override' wins over the generated one and is never overwritten by later runs.`,
	}

	var listLimit int
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List glossary entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			entries, err := client.ListGlossaryEntries(listLimit)
			if err != nil {
				return fmt.Errorf("list glossary: %w", err)
			}

			if jsonOutput {
				out, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return fmt.Errorf("marshal JSON: %w", err)
				}
				fmt.Println(string(out))
				return nil
			}

			if len(entries) == 0 {
				fmt.Println("No glossary entries yet. Generate a digest with --glossary first.")
				return nil
			}

			tw := newTable("TERM", "KIND", "TYPE", "OVR", "DEFINITION")
			for _, e := range entries {
				ovr := ""
				if e.ManualOverride {
					ovr = styleOK.Render("✎")
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", e.Term, e.Kind, e.Category, ovr, truncate(e.EffectiveDefinition(), 80))
			}
			tw.Flush()
			return nil
		},
	}
	listCmd.Flags().IntVar(&listLimit, "limit", 0, "Max entries to show (0 = all)")

	overrideCmd := &cobra.Command{
		Use:   "override <term> <definition>",
		Short: "Set a curated definition that survives regeneration",
		Long: `Set a human-written definition for a glossary term. It wins over the LLM-generated
definition and is never overwritten when digests are regenerated. The term is matched
case-insensitively and is space/hyphen-insensitive (e.g. "cobalt strike" == "cobalt-strike").`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			term := args[0]
			definition := strings.Join(args[1:], " ")

			entry, err := client.SetGlossaryOverride(term, definition)
			if err != nil {
				return fmt.Errorf("set override: %w", err)
			}

			fmt.Printf("%s Curated definition set for %q:\n  %s\n", styleOK.Render("✓"), entry.Term, entry.EffectiveDefinition())
			return nil
		},
	}

	glossaryCmd.AddCommand(listCmd, overrideCmd)
	return glossaryCmd
}
