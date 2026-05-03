package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

func createAuthLoginCommand() *cobra.Command {
	var providerName string
	var modelName string

	cmd := &cobra.Command{
		Use:   "login openai-codex",
		Short: "Register a ChatGPT/Codex subscription via device-code OAuth",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "openai-codex" {
				return fmt.Errorf("unsupported provider %q — only openai-codex is supported", args[0])
			}

			client := getNewDownlinkClient()
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

	cmd.Flags().StringVar(&providerName, "provider-name", "codex-sub",
		"Name of the openai-codex provider config entry (created if missing)")
	cmd.Flags().StringVar(&modelName, "model-name", "codex-mini",
		"Model name to use when auto-creating the provider entry")

	return cmd
}
