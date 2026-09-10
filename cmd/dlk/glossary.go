package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/huh/v2"
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

	var deleteYes bool
	deleteCmd := &cobra.Command{
		Use:   "delete [term]",
		Short: "Remove entries from the glossary",
		Long: `Delete glossary entries and every digest reference to them.

With a term, deletes that entry directly. Without one, opens a filterable picker over the whole
glossary — type to narrow, space to select, enter to confirm — and a leading argument can also
be used as a search query to pre-narrow the list.

Definitions are generated once and never regenerated, so deleting is how a term with a wrong
cached definition gets reconsidered from scratch on a later digest. Terms matched the same way
as 'override': case-insensitively and space/hyphen-insensitively.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			var keys []string
			if len(args) == 1 && deleteYes {
				// Unattended: treat the argument as the exact term, never as a search query.
				keys = []string{args[0]}
			} else {
				query := ""
				if len(args) == 1 {
					query = args[0]
				}
				selected, err := selectGlossaryEntries(client, query)
				if err != nil {
					return err
				}
				if len(selected) == 0 {
					fmt.Println("Cancelled.")
					return nil
				}
				keys = selected

				noun := "entries"
				if len(keys) == 1 {
					noun = "entry"
				}
				confirm := false
				flushStdin()
				if err := huh.NewConfirm().
					Title(fmt.Sprintf("Delete %d glossary %s?", len(keys), noun)).
					Affirmative("Yes, delete").
					Negative("No, keep them").
					Value(&confirm).
					WithTheme(dlkPromptTheme).Run(); err != nil || !confirm {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			for _, key := range keys {
				term, err := client.DeleteGlossaryEntry(key)
				if err != nil {
					return fmt.Errorf("delete %q: %w", key, err)
				}
				fmt.Printf("%s Deleted %q\n", styleOK.Render("✓"), term)
			}
			return nil
		},
	}
	deleteCmd.Flags().BoolVarP(&deleteYes, "yes", "y", false, "Delete the given term without the picker or a confirmation")

	glossaryCmd.AddCommand(listCmd, overrideCmd, deleteCmd)
	return glossaryCmd
}
