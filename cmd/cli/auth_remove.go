package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func createAuthRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove <provider-name> <credential-id>",
		Short: "Remove a Codex credential by ID",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()
			resp, err := client.RemoveCodexCredential(args[0], args[1])
			if err != nil {
				return fmt.Errorf("remove credential: %w", err)
			}
			if resp.Removed {
				fmt.Printf("Credential %s removed from provider %s\n", args[1], args[0])
			} else {
				fmt.Printf("Credential %s not found in provider %s\n", args[1], args[0])
			}
			return nil
		},
	}
	return cmd
}
