package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ma111e/downlink/pkg/downlinkclient"
	"github.com/ma111e/downlink/pkg/models"

	"charm.land/huh/v2"
	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

// Model commands
func createModelCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "model",
		Short: "Select and manage LLM models",
		Long:  `Select the active LLM model for analysis, or manage provider configurations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			// Get all configured providers
			providers, err := client.GetLLMProviders()
			if err != nil {
				return fmt.Errorf("fetch providers: %w", err)
			}
			if len(providers) == 0 {
				return fmt.Errorf("no providers configured; use 'model add' first")
			}

			// Show currently active provider/model and find its index for pre-selection
			selectedIdx := -1
			if analysisConfig, err := client.GetAnalysisConfig(); err == nil && analysisConfig.Provider != "" {
				for i, p := range providers {
					if p.Name == analysisConfig.Provider {
						if p.ModelName != "" {
							fmt.Printf("Current: %s - %s\n", p.Name, p.ModelName)
						} else {
							fmt.Printf("Current: %s\n", p.Name)
						}
						selectedIdx = i
						break
					}
				}
			}

			// Build provider picker options: show "name (type): current model"
			providerOpts := make([]huh.Option[int], len(providers))
			for i, p := range providers {
				label := p.Name
				if p.ProviderType != "" {
					label = fmt.Sprintf("%s (%s)", p.Name, p.ProviderType)
				}
				if p.ModelName != "" {
					label = fmt.Sprintf("%s — %s", label, p.ModelName)
				}
				providerOpts[i] = huh.NewOption(label, i)
			}
			flushStdin()
			if err := huh.NewSelect[int]().
				Title("Select LLM provider").
				Options(providerOpts...).
				Value(&selectedIdx).
				WithTheme(dlkPromptTheme).Run(); err != nil {
				return nil
			}

			if selectedIdx < 0 || selectedIdx >= len(providers) {
				return fmt.Errorf("invalid selection")
			}

			selected := providers[selectedIdx]

			// Load available models for the selected provider
			modelName := resolveModelInteractive(client, selected.Name, selected.ProviderType, selected.BaseURL)
			if modelName == "" {
				return nil // user cancelled
			}

			// Save the model selection
			// 1. Update provider's ModelName
			providers[selectedIdx].ModelName = modelName
			if err := client.SaveLLMProviders(providers); err != nil {
				return fmt.Errorf("save provider config: %w", err)
			}

			// 2. Load existing analysis config to preserve Persona/Workers
			analysisConfig, err := client.GetAnalysisConfig()
			if err != nil {
				return fmt.Errorf("fetch analysis config: %w", err)
			}

			// 3. Update analysis config with the new provider
			analysisConfig.Provider = selected.Name
			if err := client.UpdateAnalysisConfig(analysisConfig); err != nil {
				return fmt.Errorf("save analysis config: %w", err)
			}

			fmt.Printf("%s Active model: %s via %s\n",
				styleOK.Render("✓"), modelName, selected.Name)
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List configured LLM providers",
		Long:    `Display all configured LLM provider entries.`,
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

	// Set persona command
	setPersonaCmd := &cobra.Command{
		Use:   "set-persona",
		Short: "Set analysis persona prompt",
		Long:  `Set or update the persona, a custom prompt prefix injected before analysis requests.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			// Load current config to preserve Provider and Workers
			config, err := client.GetAnalysisConfig()
			if err != nil {
				return fmt.Errorf("fetch analysis config: %w", err)
			}

			persona := config.Persona
			flushStdin()
			if err := huh.NewText().
				Title("Persona").
				Description("Prompt prefix injected before every analysis request (leave blank to clear)").
				Value(&persona).
				WithTheme(dlkPromptTheme).Run(); err != nil {
				return err
			}

			// Update with trimmed persona, preserving other fields
			config.Persona = strings.TrimSpace(persona)
			if err := client.UpdateAnalysisConfig(config); err != nil {
				return fmt.Errorf("save analysis config: %w", err)
			}

			fmt.Printf("%s Persona updated\n", styleOK.Render("✓"))
			return nil
		},
	}

	// Save LLM providers command
	var providerType, modelName, apiKey, baseURLFlag string
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
				}
			}
		},
	}
	// Add flags for configure command
	saveProvidersCmd.Flags().StringVarP(&providerType, "provider", "p", "", "Provider type (openai, anthropic, ollama, mistral)")
	saveProvidersCmd.Flags().StringVarP(&modelName, "model", "m", "", "Model name to use")
	saveProvidersCmd.Flags().StringVarP(&apiKey, "api-key", "k", "", "API key for the provider")
	saveProvidersCmd.Flags().StringVarP(&baseURLFlag, "url", "u", "", "Base URL for the provider endpoint")
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

	cmd.AddCommand(listCmd, setPersonaCmd, saveProvidersCmd, addProviderCmd, removeProviderCmd, createCredsCommands())
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
			huh.NewOption("claude-code", "claude-code"),
		).
		Value(&providerType).
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Step 3: API key or base URL (conditional on provider type)
	var apiKey, baseURL string
	switch providerType {
	case "claude-code":
		// claude-code authenticates via OAuth, not an API key. The credential is
		// attached separately by the login flow; tie it to this entry's name.
		fmt.Printf("\nclaude-code uses OAuth — no API key needed here.\nAfter adding, run: dlk model creds login claude-code --provider-name %s\n\n", name)
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
			WithTheme(dlkPromptTheme).Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
		flushStdin()
		if err := huh.NewInput().
			Title("Base URL").
			Description("Leave empty to use the provider's default endpoint").
			Placeholder("e.g. https://api.openai.com/v1").
			Value(&baseURL).
			WithTheme(dlkPromptTheme).Run(); err != nil {
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
			WithTheme(dlkPromptTheme).Run(); err != nil {
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
			WithTheme(dlkPromptTheme).Run(); err != nil {
			fmt.Println("Cancelled.")
			return
		}
	}

	// Step 4: Model selection (new provider not yet saved, so no name filter)
	modelName := resolveModelInteractive(client, "", providerType, baseURL)
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil || !confirm {
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

// claudeCodeModelIDs returns the known Claude model IDs offered by the
// claude-code subscription, newest first. Used by the add-provider wizard, which
// runs before any OAuth credential exists to fetch a live list. Users can always
// pick "Custom..." for a model not listed here.
func claudeCodeModelIDs() []string {
	return []string{
		"claude-opus-4-8",
		"claude-sonnet-4-6",
		"claude-haiku-4-5",
	}
}

// resolveModelInteractive fetches available models for the provider and lets the user pick one.
// Falls back to a free-text input if the fetch fails or returns no results.
func resolveModelInteractive(client *downlinkclient.DownlinkClient, providerName, providerType, baseURL string) string {
	fmt.Println("Fetching available models...")

	var modelList []string

	// claude-code: offer the known Claude model IDs. The entry is usually created
	// before any OAuth credential exists, so we cannot fetch a live list here.
	if strings.EqualFold(providerType, "claude-code") {
		modelList = claudeCodeModelIDs()
	} else if strings.EqualFold(providerType, "openai-codex") {
		// Fetch provider configs to get stored credentials
		providers, err := client.GetLLMProviders()
		if err != nil {
			fmt.Println("Error: Could not fetch provider credentials from server")
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
				WithTheme(dlkPromptTheme).Run()
			return strings.TrimSpace(modelName)
		}

		// Find the Codex provider config to get stored credentials
		var codexProvider *models.ProviderConfig
		for i := range providers {
			if strings.EqualFold(providers[i].ProviderType, "openai-codex") {
				codexProvider = &providers[i]
				break
			}
		}

		if codexProvider == nil || len(codexProvider.Credentials) == 0 {
			fmt.Println("Error: No Codex credentials stored. Run 'dlk model creds login' to authenticate.")
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
				WithTheme(dlkPromptTheme).Run()
			return strings.TrimSpace(modelName)
		}

		// Use the first credential (highest priority) to fetch models
		accessToken := codexProvider.Credentials[0].AccessToken
		modelList = getCodexModelIDs(accessToken)
		fmt.Printf("Found %d Codex models\n", len(modelList))
	} else {
		// Standard provider: use server-provided models
		resp, err := client.GetAvailableModelsForProvider(providerName, providerType, baseURL)

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
				WithTheme(dlkPromptTheme).Run()
			return strings.TrimSpace(modelName)
		}

		// Convert server model list to string slugs
		for _, m := range resp.Models {
			modelList = append(modelList, m.Name)
		}
	}

	if len(modelList) == 1 {
		fmt.Printf("Auto-selected model: %s\n", modelList[0])
		return modelList[0]
	}

	const customVal = "__custom__"
	options := make([]huh.Option[string], 0, len(modelList)+1)
	for _, model := range modelList {
		options = append(options, huh.NewOption(model, model))
	}
	options = append(options, huh.NewOption("Custom...", customVal))

	var modelChoice string
	flushStdin()
	if err := huh.NewSelect[string]().
		Title("Model").
		Options(options...).
		Value(&modelChoice).
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run()
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
		fmt.Println("Cancelled.")
		return
	}

	// Enabled
	enabled := p.Enabled
	flushStdin()
	if err := huh.NewConfirm().
		Title("Enable this provider?").
		Value(&enabled).
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil {
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
		WithTheme(dlkPromptTheme).Run(); err != nil || !confirm {
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
