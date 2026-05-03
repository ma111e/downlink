package main

import "github.com/spf13/cobra"

func createAuthCommands() *cobra.Command {
	authCmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage provider authentication credentials",
	}

	authCmd.AddCommand(createAuthLoginCommand())
	authCmd.AddCommand(createAuthListCommand())
	authCmd.AddCommand(createAuthRemoveCommand())
	authCmd.AddCommand(createAuthPriorityCommand())

	return authCmd
}
