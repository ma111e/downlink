package main

import (
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func createAuthRemoveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [provider] [credential-id]",
		Short: "Remove a credential by ID",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			var providerName, credentialID string

			if len(args) >= 1 {
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

			if len(args) >= 2 {
				credentialID = args[1]
			} else {
				var err error
				credentialID, err = selectCredential(client, providerName)
				if err != nil {
					return err
				}
				if credentialID == "" {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			confirm := false
			flushStdin()
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Remove credential %s from provider %s?", credentialID, providerName)).
				Affirmative("Yes, remove").
				Negative("No, keep it").
				Value(&confirm).
				Run(); err != nil || !confirm {
				fmt.Println("Cancelled.")
				return nil
			}

			resp, err := client.RemoveCodexCredential(providerName, credentialID)
			if err != nil {
				return fmt.Errorf("remove credential: %w", err)
			}
			if resp.Removed {
				fmt.Printf("Credential %s removed from provider %s\n", credentialID, providerName)
			} else {
				fmt.Printf("Credential %s not found in provider %s\n", credentialID, providerName)
			}
			return nil
		},
	}
	return cmd
}
