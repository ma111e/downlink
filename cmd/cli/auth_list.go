package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func createAuthListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [provider]",
		Short: "List registered credentials for a provider",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			var providerName string
			if len(args) > 0 {
				providerName = args[0]
			} else {
				var err error
				providerName, err = selectOAuthProvider(client)
				if err != nil {
					return err
				}
				if providerName == "" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			resp, err := client.ListCodexCredentials(providerName)
			if err != nil {
				return fmt.Errorf("list credentials: %w", err)
			}

			if len(resp.Credentials) == 0 {
				fmt.Printf("No credentials registered for provider %q\n", providerName)
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
