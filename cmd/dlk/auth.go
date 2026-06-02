package main

import (
	"fmt"

	"charm.land/huh/v2"
	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/spf13/cobra"
)

// oauthProviderTypes lists provider types that support device-code OAuth login.
// Extend this slice when adding new OAuth-capable providers.
var oauthProviderTypes = []string{"openai-codex"}

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

// selectOAuthProvider prompts the user to pick one of their configured OAuth-capable
// provider entries. Returns the config entry name (e.g. "codex-sub").
// Returns ("", nil) when the user cancels.
func selectOAuthProvider(client *downlinkclient.DownlinkClient) (string, error) {
	providers, err := client.GetLLMProviders()
	if err != nil {
		return "", fmt.Errorf("failed to list providers: %w", err)
	}

	typeSet := make(map[string]bool, len(oauthProviderTypes))
	for _, t := range oauthProviderTypes {
		typeSet[t] = true
	}

	type entry struct{ name, providerType string }
	var capable []entry
	for _, p := range providers {
		if typeSet[p.ProviderType] {
			capable = append(capable, entry{p.Name, p.ProviderType})
		}
	}

	if len(capable) == 0 {
		return "", fmt.Errorf("no OAuth providers configured — run 'auth login' first")
	}
	if len(capable) == 1 {
		return capable[0].name, nil
	}

	opts := make([]huh.Option[string], len(capable))
	for i, p := range capable {
		opts[i] = huh.NewOption(fmt.Sprintf("%s  (%s)", p.name, p.providerType), p.name)
	}

	var selected string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Select provider").
		Options(opts...).
		Value(&selected).
		Run(); err != nil {
		return "", nil
	}
	return selected, nil
}

// selectCredential prompts the user to pick a credential for the given provider.
// Returns the credential ID. Returns ("", nil) when the user cancels.
func selectCredential(client *downlinkclient.DownlinkClient, providerName string) (string, error) {
	resp, err := client.ListCodexCredentials(providerName)
	if err != nil {
		return "", fmt.Errorf("failed to list credentials: %w", err)
	}
	if len(resp.Credentials) == 0 {
		return "", fmt.Errorf("no credentials registered for provider %q", providerName)
	}
	if len(resp.Credentials) == 1 {
		return resp.Credentials[0].Id, nil
	}

	opts := make([]huh.Option[string], len(resp.Credentials))
	for i, c := range resp.Credentials {
		status := c.LastStatus
		if status == "" {
			status = "ok"
		}
		label := fmt.Sprintf("%-10s  %-30s  priority: %d  (%s)", c.Id, c.Label, c.Priority, status)
		opts[i] = huh.NewOption(label, c.Id)
	}

	var selected string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Select credential").
		Options(opts...).
		Value(&selected).
		Run(); err != nil {
		return "", nil
	}
	return selected, nil
}
