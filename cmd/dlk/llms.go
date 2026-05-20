package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"downlink/pkg/downlinkclient"
	"downlink/pkg/models"

	"charm.land/huh/v2"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

// Model commands
func createModelCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Manage LLM provider configurations",
		Long:  `Add, remove, and configure LLM provider entries.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List LLM resources",
		Long:  `List configured providers or available models.`,
	}

	listProvidersCmd := &cobra.Command{
		Use:     "providers",
		Aliases: []string{"profiles"},
		Short:   "List configured LLM providers",
		Long:    `Display all configured LLM provider entries. "profiles" is accepted as an alias.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			providers, err := client.GetLLMProviders()
			if err != nil {
				fmt.Printf("Error getting LLM providers: %v\n", err)
				return
			}
			if len(providers) == 0 {
				fmt.Println("No LLM providers configured.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(providers, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printProviderTable(providers)
			}
		},
	}

	listModelsCmd := &cobra.Command{
		Use:   "models",
		Short: "List available LLM models",
		Long:  `Display models available from all configured providers.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			availableModels, err := client.GetAvailableModels()
			if err != nil {
				fmt.Printf("Error getting available models: %v\n", err)
				return
			}
			if availableModels == nil || len(availableModels.Models) == 0 {
				fmt.Println("No available models returned.")
				return
			}

			if jsonOutput {
				out, err := json.MarshalIndent(availableModels, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
			} else {
				printModelInfoTable(availableModels.Models)
			}
		},
	}

	listCmd.AddCommand(listProvidersCmd, listModelsCmd)

	// Save LLM providers command
	var providerType, modelName, apiKey, baseURLFlag string
	var temperature float64
	var enabled bool
	var updateAllProviders bool
	var inputFile string

	saveProvidersCmd := &cobra.Command{
		Use:   "update",
		Short: "Update LLM providers",
		Long:  `Update LLM provider configurations`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			// No targeting flags: run interactive flow
			if !cmd.Flags().Changed("provider") && !cmd.Flags().Changed("all") && !cmd.Flags().Changed("file") {
				runUpdateProviderInteractive(client)
				return
			}

			// Get existing providers
			providers, err := client.GetLLMProviders()
			if err != nil {
				fmt.Printf("Error getting LLM providers: %v\n", err)
				return
			}
			config, err := client.GetConfig()
			if err != nil {
				fmt.Printf("Error getting config: %v\n", err)
				return
			}

			// Check if input file was provided for bulk configuration
			if inputFile != "" {
				data, err := os.ReadFile(inputFile)
				if err != nil {
					fmt.Printf("Error reading configuration file: %v\n", err)
					return
				}

				var newProviders []models.ProviderConfig

				if err := json.Unmarshal(data, &newProviders); err != nil {
					fmt.Printf("Error parsing configuration file: %v\n", err)
					return
				}

				// Update providers from file
				for _, np := range newProviders {
					updated := false
					for i, p := range providers {
						if p.ProviderType == np.ProviderType {
							providers[i].ModelName = np.ModelName
							providers[i].Enabled = np.Enabled
							providers[i].Temperature = np.Temperature
							updated = true
							break
						}
					}

					if !updated {
						// Add new provider
						providers = append(providers, np)
					}
				}
			} else if updateAllProviders {
				// Update all providers with common settings
				for i := range providers {
					if modelName != "" {
						providers[i].ModelName = modelName
					}
					providers[i].Enabled = enabled
					if temperature != 0 {
						providers[i].Temperature = &temperature
					}
				}
			} else if providerType != "" {
				// Update specific provider
				providerFound := false
				for i, p := range providers {
					if strings.EqualFold(p.ProviderType, providerType) {
						providerFound = true

						// Update model name if provided
						if modelName != "" {
							providers[i].ModelName = modelName
						}

						// Update enabled status
						providers[i].Enabled = enabled

						// Update temperature if provided
						if temperature != 0 {
							providers[i].Temperature = &temperature
						}

						// Update base URL if provided
						if baseURLFlag != "" {
							providers[i].BaseURL = baseURLFlag
						}

						break
					}
				}

				if !providerFound {
					// Add new provider
					newProvider := models.ProviderConfig{
						ProviderType: providerType,
						ModelName:    modelName,
						Enabled:      enabled,
						Temperature:  &temperature,
						BaseURL:      baseURLFlag,
					}
					providers = append(providers, newProvider)
				}
			}

			// Update API key in the provider config if provided
			if apiKey != "" && providerType != "" {
				updated := false
				for i := range config.Providers {
					if strings.EqualFold(config.Providers[i].ProviderType, providerType) {
						config.Providers[i].APIKey = apiKey
						updated = true
						break
					}
				}

				if !updated {
					log.WithFields(log.Fields{
						"providerType": providerType,
					}).Warn("Provider not found in config, API key not saved")
				} else {
					// Save updated config
					err := client.SaveConfig(config)
					if err != nil {
						log.WithFields(log.Fields{
							"providerType": providerType,
						}).Error("Failed to save config")
						return
					}

					log.WithFields(log.Fields{
						"providerType": providerType,
					}).Info("Updated API key for provider")
				}
			}

			// Save updated providers
			err = client.SaveLLMProviders(providers)
			if err != nil {
				log.WithFields(log.Fields{
					"providerType": providerType,
				}).Error("Failed to save LLM providers")
				return
			}

			fmt.Println("Successfully updated LLM provider configurations")

			// Display updated configuration
			if jsonOutput {
				out, err := json.MarshalIndent(providers, "", "  ")
				if err != nil {
					log.WithFields(log.Fields{
						"error": err,
					}).Error("Failed to marshal providers to JSON")
					return
				}
				fmt.Println(string(out))
			} else {
				fmt.Println("Updated LLM Providers:")
				for i, provider := range providers {
					fmt.Printf("%d. %s/(%s) - Enabled: %v\n", i+1, provider.ProviderType, provider.ModelName, provider.Enabled)
					if provider.Temperature != nil {
						fmt.Printf("   Temperature: %.2f\n", *provider.Temperature)
					}
				}
			}
		},
	}
	// Add flags for configure command
	saveProvidersCmd.Flags().StringVarP(&providerType, "provider", "p", "", "Provider type (openai, anthropic, ollama, mistral)")
	saveProvidersCmd.Flags().StringVarP(&modelName, "model", "m", "", "Model name to use")
	saveProvidersCmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "API key for the provider")
	saveProvidersCmd.Flags().StringVarP(&baseURLFlag, "url", "u", "", "Base URL for the provider endpoint")
	saveProvidersCmd.Flags().Float64VarP(&temperature, "temperature", "t", 0, "Temperature setting (0.0-1.0)")
	saveProvidersCmd.Flags().BoolVarP(&enabled, "enabled", "e", true, "Enable or disable the provider")
	saveProvidersCmd.Flags().BoolVarP(&updateAllProviders, "all", "a", false, "Update all providers with the same settings")
	saveProvidersCmd.Flags().StringVarP(&inputFile, "file", "f", "", "JSON file containing provider configurations")

	addProviderCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new LLM provider configuration",
		Long:  `Interactively create a new LLM provider entry.`,
		Run:   runAddProvider,
	}

	removeProviderCmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove an LLM provider configuration",
		Long:  `Interactively select and remove an existing LLM provider entry.`,
		Run:   runRemoveProvider,
	}

	cmd.AddCommand(listCmd, saveProvidersCmd, addProviderCmd, removeProviderCmd)
	return cmd
}

func runAddProvider(cmd *cobra.Command, args []string) {
	client := getNewDownlinkClient()

	existing, err := client.GetLLMProviders()
	if err != nil {
		fmt.Printf("Error fetching providers: %v\n", err)
		return
	}

	// Step 1: Provider type
	var providerType string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Provider type").
		Options(
			huh.NewOption("openai", "openai"),
			huh.NewOption("anthropic", "anthropic"),
			huh.NewOption("mistral", "mistral"),
			huh.NewOption("ollama", "ollama"),
			huh.NewOption("llamacpp", "llamacpp"),
			huh.NewOption("openai-codex", "openai-codex"),
		).
		Value(&providerType).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Step 2: Name (unique)
	var name string
	flushStdin()
	if err := huh.NewInput().
		Title("Name").
		Description("A unique identifier for this provider entry").
		Value(&name).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("name is required")
			}
			for _, p := range existing {
				if p.Name == s {
					return fmt.Errorf("a provider named %q already exists", s)
				}
			}
			return nil
		}).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Step 3: API key or base URL (conditional on provider type)
	var apiKey, baseURL string
	switch providerType {
	case "openai", "anthropic", "mistral", "openai-codex":
		flushStdin()
		if err := huh.NewInput().
			Title("API key").
			EchoMode(huh.EchoModePassword).
			Value(&apiKey).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("API key is required")
				}
				return nil
			}).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
		flushStdin()
		if err := huh.NewInput().
			Title("Base URL").
			Description("Leave empty to use the provider's default endpoint").
			Placeholder("e.g. https://api.openai.com/v1").
			Value(&baseURL).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
		baseURL = strings.TrimSpace(baseURL)
	case "ollama":
		baseURL = "http://localhost:11434"
		flushStdin()
		if err := huh.NewInput().
			Title("Base URL").
			Value(&baseURL).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
	case "llamacpp":
		flushStdin()
		if err := huh.NewInput().
			Title("Base URL").
			Value(&baseURL).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("base URL is required for llamacpp")
				}
				return nil
			}).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
	}

	// Step 4: Model selection
	modelName := resolveModelInteractive(client, providerType, baseURL)
	if modelName == "" {
		fmt.Println("Cancelled.")
		return
	}

	// Step 5: Timeout minutes (optional)
	var timeoutStr string
	flushStdin()
	if err := huh.NewInput().
		Title("Timeout (minutes)").
		Placeholder("20").
		Description("Press Enter to use the default (20 minutes)").
		Value(&timeoutStr).
		Validate(func(s string) error {
			if s == "" {
				return nil
			}
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n <= 0 {
				return fmt.Errorf("must be a positive integer")
			}
			return nil
		}).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}
	var timeoutMinutes *int
	if s := strings.TrimSpace(timeoutStr); s != "" {
		n, _ := strconv.Atoi(s)
		timeoutMinutes = &n
	}

	// Step 6: Enable?
	enabled := true
	flushStdin()
	if err := huh.NewConfirm().
		Title("Enable this provider?").
		Value(&enabled).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Summary
	fmt.Println()
	tw := newTable("FIELD", "VALUE")
	fmt.Fprintf(tw, "Name\t%s\n", name)
	fmt.Fprintf(tw, "Type\t%s\n", providerType)
	fmt.Fprintf(tw, "Model\t%s\n", modelName)
	if apiKey != "" {
		fmt.Fprintf(tw, "API key\t%s\n", mask(apiKey))
	}
	if baseURL != "" {
		fmt.Fprintf(tw, "Base URL\t%s\n", baseURL)
	}
	if timeoutMinutes != nil {
		fmt.Fprintf(tw, "Timeout\t%dm\n", *timeoutMinutes)
	} else {
		fmt.Fprintf(tw, "Timeout\t20m (default)\n")
	}
	fmt.Fprintf(tw, "Enabled\t%v\n", enabled)
	tw.Flush()
	fmt.Println()

	// Final confirmation
	confirm := true
	flushStdin()
	if err := huh.NewConfirm().
		Title("Add this provider?").
		Value(&confirm).
		Run(); err != nil || !confirm {
		fmt.Println("Aborted.")
		return
	}

	newProvider := models.ProviderConfig{
		Name:           name,
		ProviderType:   providerType,
		ModelName:      modelName,
		APIKey:         apiKey,
		BaseURL:        baseURL,
		TimeoutMinutes: timeoutMinutes,
		Enabled:        enabled,
	}

	if err := client.SaveLLMProviders(append(existing, newProvider)); err != nil {
		fmt.Printf("Error saving provider: %v\n", err)
		return
	}
	fmt.Printf("✓ Provider %q added.\n", name)
}

// resolveModelInteractive fetches available models for the provider and lets the user pick one.
// Falls back to a free-text input if the fetch fails or returns no results.
func resolveModelInteractive(client *downlinkclient.DownlinkClient, providerType, baseURL string) string {
	fmt.Println("Fetching available models...")
	resp, err := client.GetAvailableModelsForProvider(providerType, baseURL)

	if err != nil || resp == nil || len(resp.Models) == 0 {
		var modelName string
		flushStdin()
		_ = huh.NewInput().
			Title("Model name").
			Placeholder("e.g. gpt-4o").
			Value(&modelName).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("model name is required")
				}
				return nil
			}).
			Run()
		return strings.TrimSpace(modelName)
	}

	if len(resp.Models) == 1 {
		m := resp.Models[0]
		fmt.Printf("Auto-selected model: %s\n", m.Name)
		return m.Name
	}

	const customVal = "__custom__"
	options := make([]huh.Option[string], 0, len(resp.Models)+1)
	for _, m := range resp.Models {
		label := m.Name
		if m.DisplayName != "" && m.DisplayName != m.Name {
			label = fmt.Sprintf("%s (%s)", m.Name, m.DisplayName)
		}
		options = append(options, huh.NewOption(label, m.Name))
	}
	options = append(options, huh.NewOption("Custom...", customVal))

	var modelChoice string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Model").
		Options(options...).
		Value(&modelChoice).
		Run(); err != nil {
		return ""
	}

	if modelChoice != customVal {
		return modelChoice
	}

	var customModel string
	flushStdin()
	_ = huh.NewInput().
		Title("Model name").
		Value(&customModel).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("model name is required")
			}
			return nil
		}).
		Run()
	return strings.TrimSpace(customModel)
}

func runUpdateProviderInteractive(client *downlinkclient.DownlinkClient) {
	providers, err := client.GetLLMProviders()
	if err != nil {
		fmt.Printf("Error fetching providers: %v\n", err)
		return
	}
	if len(providers) == 0 {
		fmt.Println("No providers configured.")
		return
	}

	// Select provider
	options := make([]huh.Option[int], len(providers))
	for i, p := range providers {
		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		options[i] = huh.NewOption(fmt.Sprintf("%s  (%s / %s / %s)", p.Name, p.ProviderType, p.ModelName, status), i)
	}

	var selectedIdx int
	flushStdin()
	if err := huh.NewSelect[int]().
		Title("Select provider to update").
		Options(options...).
		Value(&selectedIdx).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	p := providers[selectedIdx]

	// Model
	modelName := p.ModelName
	flushStdin()
	if err := huh.NewInput().
		Title("Model name").
		Value(&modelName).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("model name is required")
			}
			return nil
		}).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// API key (leave blank to keep current)
	apiKey := ""
	flushStdin()
	if err := huh.NewInput().
		Title("API key").
		Description("Leave empty to keep the current key").
		EchoMode(huh.EchoModePassword).
		Value(&apiKey).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Base URL
	baseURL := p.BaseURL
	flushStdin()
	if err := huh.NewInput().
		Title("Base URL").
		Description("Leave empty to keep the current value (blank = provider default)").
		Value(&baseURL).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Enabled
	enabled := p.Enabled
	flushStdin()
	if err := huh.NewConfirm().
		Title("Enable this provider?").
		Value(&enabled).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	providers[selectedIdx].ModelName = strings.TrimSpace(modelName)
	providers[selectedIdx].BaseURL = strings.TrimSpace(baseURL)
	providers[selectedIdx].Enabled = enabled
	if k := strings.TrimSpace(apiKey); k != "" {
		providers[selectedIdx].APIKey = k
	}

	if err := client.SaveLLMProviders(providers); err != nil {
		fmt.Printf("Error saving providers: %v\n", err)
		return
	}
	fmt.Printf("✓ Provider %q updated.\n", p.Name)
}

func runRemoveProvider(cmd *cobra.Command, args []string) {
	client := getNewDownlinkClient()

	providers, err := client.GetLLMProviders()
	if err != nil {
		fmt.Printf("Error fetching providers: %v\n", err)
		return
	}
	if len(providers) == 0 {
		fmt.Println("No providers configured.")
		return
	}

	options := make([]huh.Option[int], len(providers))
	for i, p := range providers {
		status := "disabled"
		if p.Enabled {
			status = "enabled"
		}
		label := fmt.Sprintf("%s  (%s / %s / %s)", p.Name, p.ProviderType, p.ModelName, status)
		options[i] = huh.NewOption(label, i)
	}

	var selectedIdx int
	flushStdin()
	if err := huh.NewSelect[int]().
		Title("Select provider to remove").
		Options(options...).
		Value(&selectedIdx).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	selected := providers[selectedIdx]
	fmt.Printf("\nProvider: %s  (%s / %s)\n\n", selected.Name, selected.ProviderType, selected.ModelName)

	confirm := false
	flushStdin()
	if err := huh.NewConfirm().
		Title(fmt.Sprintf("Remove %q?", selected.Name)).
		Affirmative("Yes, remove").
		Negative("No, keep it").
		Value(&confirm).
		Run(); err != nil || !confirm {
		fmt.Println("Cancelled.")
		return
	}

	updated := make([]models.ProviderConfig, 0, len(providers)-1)
	for i, p := range providers {
		if i != selectedIdx {
			updated = append(updated, p)
		}
	}

	if err := client.SaveLLMProviders(updated); err != nil {
		fmt.Printf("Error saving providers: %v\n", err)
		return
	}
	fmt.Printf("✓ Provider %q removed.\n", selected.Name)
}
