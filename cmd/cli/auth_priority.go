package main

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

func createAuthPriorityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "priority <provider-name> <credential-id> <priority>",
		Short: "Set priority for a Codex credential (lower = preferred)",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := strconv.Atoi(args[2])
			if err != nil {
				return fmt.Errorf("invalid priority %q: must be an integer", args[2])
			}
			client := getNewDownlinkClient()
			resp, err := client.SetCodexCredentialPriority(args[0], args[1], int32(p))
			if err != nil {
				return fmt.Errorf("set priority: %w", err)
			}
			if resp.Updated {
				fmt.Printf("Priority for credential %s set to %d\n", args[1], p)
			} else {
				fmt.Printf("Credential %s not found in provider %s\n", args[1], args[0])
			}
			return nil
		},
	}
	return cmd
}
