package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"downlink/pkg/models"

	log "github.com/sirupsen/logrus"

	"github.com/davecgh/go-spew/spew"
	"github.com/spf13/cobra"
)

// LLM commands
func createLLMCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "llm",
		Short: "Manage LLM providers",
		Long:  `Configure and manage LLM providers and models.`,
	}

	// Combined list command that replaces both providers and models commands
	listCmd := &cobra.Command{
		Use:     "list [resource]",
		Aliases: []string{"ls"},
		Short:   "List LLM resources",
		Long:    `List LLM resources (providers or models).`,
		Args:    cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			resource := strings.ToLower(args[0])

			switch resource {
			case "providers", "profiles":
				providers, err := client.GetLLMProviders()
				if err != nil {
					fmt.Printf("Error getting LLM providers: %v\n", err)
					return
				}
				if len(providers) == 0 {
					log.WithFields(log.Fields{
						"resource": resource,
					}).Info("No LLM providers configured.")
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
					fmt.Printf("%-20s %-12s %-36s %s\n", "NAME", "TYPE", "MODEL", "ENABLED")
					fmt.Printf("%-20s %-12s %-36s %s\n", strings.Repeat("-", 20), strings.Repeat("-", 12), strings.Repeat("-", 36), strings.Repeat("-", 7))
					for _, provider := range providers {
						enabled := "no"
						if provider.Enabled {
							enabled = "yes"
						}
						name := provider.Name
						if name == "" {
							name = "(unnamed)"
						}
						fmt.Printf("%-20s %-12s %-36s %s\n", name, provider.ProviderType, provider.ModelName, enabled)
					}
				}

			case "models":
				availableModels, err := client.GetAvailableModels()
				if err != nil {
					fmt.Printf("Error getting available models: %v\n", err)
					return
				}
				if availableModels == nil {
					log.WithFields(log.Fields{
						"resource": resource,
					}).Info("No available models returned")
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
					fmt.Println("Available Models:")

					for _, model := range availableModels.Models {
						spew.Dump(model)
					}
				}

			default:
				log.WithFields(log.Fields{
					"resource": resource,
				}).Info("Unknown resource type")
				fmt.Println("Supported resources: providers (or profiles), models")
			}
		},
	}

	// Save LLM providers command
	var providerType, modelName, apiKey string
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
	saveProvidersCmd.Flags().Float64VarP(&temperature, "temperature", "t", 0, "Temperature setting (0.0-1.0)")
	saveProvidersCmd.Flags().BoolVarP(&enabled, "enabled", "e", true, "Enable or disable the provider")
	saveProvidersCmd.Flags().BoolVarP(&updateAllProviders, "all", "a", false, "Update all providers with the same settings")
	saveProvidersCmd.Flags().StringVarP(&inputFile, "file", "f", "", "JSON file containing provider configurations")

	cmd.AddCommand(listCmd, saveProvidersCmd)
	return cmd
}
