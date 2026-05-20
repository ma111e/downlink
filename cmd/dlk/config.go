package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"downlink/pkg/models"
	"github.com/spf13/cobra"
)

// Config commands
func createConfigCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage server configuration",
		Long:  `View and update the server's configuration settings.`,
	}

	// Show config command
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Show server configuration",
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

	// Edit config command
	editCmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit server configuration in $EDITOR",
		Long:  `Open the server configuration in your editor ($EDITOR, default: vi).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := getNewDownlinkClient()

			// Get current config
			config, err := client.GetConfig()
			if err != nil {
				return fmt.Errorf("fetch config: %w", err)
			}

			// Marshal to JSON
			data, err := json.MarshalIndent(config, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal config: %w", err)
			}

			// Create temp file
			tmpFile, err := os.CreateTemp("", "downlink-config-*.json")
			if err != nil {
				return fmt.Errorf("create temp file: %w", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write config to temp file
			if _, err := tmpFile.Write(data); err != nil {
				tmpFile.Close()
				return fmt.Errorf("write temp file: %w", err)
			}
			tmpFile.Close()

			// Get editor from env, default to vi
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}

			// Open editor
			editCmd := exec.Command(editor, tmpFile.Name())
			editCmd.Stdin = os.Stdin
			editCmd.Stdout = os.Stdout
			editCmd.Stderr = os.Stderr
			if err := editCmd.Run(); err != nil {
				return fmt.Errorf("editor exited with error: %w", err)
			}

			// Read back edited file
			editedData, err := os.ReadFile(tmpFile.Name())
			if err != nil {
				return fmt.Errorf("read edited file: %w", err)
			}

			// Unmarshal back to config
			var updatedConfig models.ServerConfig
			if err := json.Unmarshal(editedData, &updatedConfig); err != nil {
				return fmt.Errorf("parse edited config: %w", err)
			}

			// Save config
			if err := client.SaveConfig(updatedConfig); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Printf("%s Configuration saved\n", styleOK.Render("✓"))
			return nil
		},
	}

	cmd.AddCommand(showCmd, editCmd)
	return cmd
}
