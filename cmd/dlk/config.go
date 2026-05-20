package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// Config commands
func createConfigCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage server configuration",
		Long:  `View and update the server's configuration settings.`,
	}

	// Get config command
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get server configuration",
		Long:  `View the current server configuration.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			config, err := client.GetConfig()
			if err != nil {
				fmt.Printf("Error getting config: %v\n", err)
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(config, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printServerConfig(config)
			}
		},
	}

	cmd.AddCommand(getCmd)
	return cmd
}
