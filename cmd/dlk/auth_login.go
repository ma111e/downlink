package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/spf13/cobra"
)

func createAuthLoginCommand() *cobra.Command {
	var providerName string

	cmd := &cobra.Command{
		Use:   "login [provider-type]",
		Short: "Register a provider subscription via device-code OAuth",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			// Step 1: Provider type, from a positional arg or interactive selector
			var providerType string
			if len(args) > 0 {
				providerType = args[0]
			} else {
				opts := make([]huh.Option[string], len(oauthProviderTypes))
				for i, t := range oauthProviderTypes {
					opts[i] = huh.NewOption(t, t)
				}
				flushStdin()
				if err := huh.NewSelect[string]().
					Title("Provider type").
					Options(opts...).
					Value(&providerType).
					Run(); err != nil {
					fmt.Println("Cancelled.")
					return nil
				}
			}

			// Step 2: Config entry name, from a flag or interactive input
			if !cmd.Flags().Changed("provider-name") {
				providerName = providerType + "-sub"
				flushStdin()
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

			// Claude Code uses a browser PKCE paste flow, not device-code.
			if providerType == "claude-code" {
				return runClaudeLogin(client, providerName)
			}

			// OAuth device-code flow
			resp, err := client.StartCodexLogin(providerName, "")
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
					return fmt.Errorf("login session expired, please try again")
				case "error":
					return fmt.Errorf("login failed: %s", poll.ErrorMessage)
				}
			}
			return fmt.Errorf("timed out waiting for login")
		},
	}

	cmd.Flags().StringVar(&providerName, "provider-name", "",
		"Name of the provider config entry to create or reuse")

	return cmd
}

// runClaudeLogin drives the Claude Code PKCE browser flow: the server returns an
// authorization URL, the user authorizes in a browser and pastes the resulting
// "<code>#<state>" string back, which the server exchanges for tokens.
func runClaudeLogin(client *downlinkclient.DownlinkClient, providerName string) error {
	resp, err := client.StartClaudeLogin(providerName, "")
	if err != nil {
		return fmt.Errorf("failed to start login: %w", err)
	}

	fmt.Printf("\nOpen this URL in your browser and authorize with your Claude Pro/Max account:\n\n  %s\n\n", resp.AuthorizeUrl)
	fmt.Println("After authorizing you'll be shown a code. Paste it below (format: code#state).")

	var code string
	flushStdin()
	if err := huh.NewInput().
		Title("Authorization code").
		Value(&code).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("code is required")
			}
			return nil
		}).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return nil
	}

	fmt.Println("\nCompleting login...")
	poll, err := client.CompleteClaudeLogin(resp.SessionId, strings.TrimSpace(code))
	if err != nil {
		return fmt.Errorf("failed to complete login: %w", err)
	}
	switch poll.Status {
	case "approved":
		fmt.Printf("\nLogin approved!\n  Provider : %s\n  Account  : %s\n  ID       : %s\n\n",
			providerName, poll.Label, poll.CredentialId)
		return nil
	default:
		return fmt.Errorf("login failed: %s", poll.ErrorMessage)
	}
}
