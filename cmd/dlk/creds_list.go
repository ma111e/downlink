package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func createCredsListCommand() *cobra.Command {
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

			tw := newTable("ID", "LABEL", "PRIORITY", "STATUS", "ERROR")
			for _, c := range resp.Credentials {
				status := c.LastStatus
				if status == "" {
					status = styleOK.Render("ok")
				} else if status == "error" {
					status = styleErr.Render(status)
				}
				fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\n",
					c.Id, c.Label, c.Priority, status, c.LastErrorReason)
			}
			tw.Flush()
			return nil
		},
	}
	return cmd
}
