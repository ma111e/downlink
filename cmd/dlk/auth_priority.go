package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func createAuthPriorityCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "priority [provider] [credential-id] [priority]",
		Short: "Set priority for a credential (lower = preferred)",
		Args:  cobra.MaximumNArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			var providerName, credentialID string
			var priority int32

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

			if len(args) >= 3 {
				p, err := strconv.Atoi(args[2])
				if err != nil {
					return fmt.Errorf("invalid priority %q: must be an integer", args[2])
				}
				priority = int32(p)
			} else {
				var priorityStr string
				flushStdin()
				if err := huh.NewInput().
					Title("Priority").
					Description("Lower value = higher priority (0 = use first)").
					Value(&priorityStr).
					Validate(func(s string) error {
						if _, err := strconv.Atoi(strings.TrimSpace(s)); err != nil {
							return fmt.Errorf("must be an integer")
						}
						return nil
					}).
					Run(); err != nil {
					fmt.Println("Cancelled.")
					return nil
				}
				p, _ := strconv.Atoi(strings.TrimSpace(priorityStr))
				priority = int32(p)
			}

			confirm := true
			flushStdin()
			if err := huh.NewConfirm().
				Title(fmt.Sprintf("Set priority for credential %s to %d?", credentialID, priority)).
				Value(&confirm).
				Run(); err != nil || !confirm {
				fmt.Println("Cancelled.")
				return nil
			}

			resp, err := client.SetCodexCredentialPriority(providerName, credentialID, priority)
			if err != nil {
				return fmt.Errorf("set priority: %w", err)
			}
			if resp.Updated {
				fmt.Printf("Priority for credential %s set to %d\n", credentialID, priority)
			} else {
				fmt.Printf("Credential %s not found in provider %s\n", credentialID, providerName)
			}
			return nil
		},
	}
	return cmd
}
