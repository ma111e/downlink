package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func createAuthListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list openai-codex",
		Short: "List registered Codex credentials for a provider",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()
			resp, err := client.ListCodexCredentials(args[0])
			if err != nil {
				return fmt.Errorf("list credentials: %w", err)
			}

			if len(resp.Credentials) == 0 {
				fmt.Printf("No credentials registered for provider %q\n", args[0])
				return nil
			}

			fmt.Printf("%-10s  %-30s  %-8s  %-12s  %s\n", "ID", "LABEL", "PRIORITY", "STATUS", "ERROR")
			for _, c := range resp.Credentials {
				status := c.LastStatus
				if status == "" {
					status = "ok"
				}
				fmt.Printf("%-10s  %-30s  %-8d  %-12s  %s\n",
					c.Id, c.Label, c.Priority, status, c.LastErrorReason)
			}
			return nil
		},
	}
	return cmd
}
