package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"
)

func createAuthLoginCommand() *cobra.Command {
	var providerName string
	var modelName string

	cmd := &cobra.Command{
		Use:   "login [provider-type]",
		Short: "Register a provider subscription via device-code OAuth",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			// Step 1: Provider type — positional arg or interactive selector
			var providerType string
			if len(args) > 0 {
				providerType = args[0]
			} else {
				opts := make([]huh.Option[string], len(oauthProviderTypes))
				for i, t := range oauthProviderTypes {
					opts[i] = huh.NewOption(t, t)
				}
				if err := huh.NewSelect[string]().
					Title("Provider type").
					Options(opts...).
					Value(&providerType).
					Run(); err != nil {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Step 2: Config entry name — flag or interactive input
			if !cmd.Flags().Changed("provider-name") {
				providerName = providerType + "-sub"
				if err := huh.NewInput().
					Title("Provider config name").
					Description("Name of the config entry to create or reuse").
					Value(&providerName).
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("name is required")
						}
						return nil
					}).
					Run(); err != nil {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Step 3: Model name — flag or interactive selection/input
			if !cmd.Flags().Changed("model-name") {
				modelName = resolveModelInteractive(client, providerType, "")
				if modelName == "" {
					return nil // user cancelled
				}
			}

			// OAuth device-code flow
			resp, err := client.StartCodexLogin(providerName, modelName)
			if err != nil {
				return fmt.Errorf("failed to start login: %w", err)
			}

			fmt.Printf("\nOpen this URL in your browser:\n  %s\n\nEnter this code:\n  %s\n\n",
				resp.VerificationUrl, resp.UserCode)
			fmt.Printf("Waiting up to %d minutes for login...\n", resp.ExpiresIn/60)

			pollInterval := time.Duration(resp.PollInterval) * time.Second
			if pollInterval < 3*time.Second {
				pollInterval = 3 * time.Second
			}

			deadline := time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second)
			for time.Now().Before(deadline) {
				time.Sleep(pollInterval)

				poll, err := client.PollCodexLogin(resp.SessionId)
				if err != nil {
					return fmt.Errorf("poll error: %w", err)
				}

				switch poll.Status {
				case "approved":
					fmt.Printf("\nLogin approved!\n  Provider : %s\n  Account  : %s\n  ID       : %s\n\n",
						providerName, poll.Label, poll.CredentialId)
					return nil
				case "pending":
					fmt.Print(".")
					continue
				case "expired":
					return fmt.Errorf("login session expired — please try again")
				case "error":
					return fmt.Errorf("login failed: %s", poll.ErrorMessage)
				}
			}
			return fmt.Errorf("timed out waiting for login")
		},
	}

	cmd.Flags().StringVar(&providerName, "provider-name", "",
		"Name of the provider config entry to create or reuse")
	cmd.Flags().StringVar(&modelName, "model-name", "",
		"Model name to associate with the provider entry")

	return cmd
}
