package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

func createQueueCommands() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "queue",
		Short: "Manage analysis queue",
		Long:  `View and control the analysis queue.`,
	}

	// Queue status command — live TUI monitor
	queueStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Monitor queue progress",
		Long:  `Live TUI monitor for the analysis queue. Press s/x/c to start/stop/clear, q to quit.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if jsonOutput {
				status := client.GetQueueStatus()
				out, err := json.MarshalIndent(status, "", "  ")
				if err != nil {
					fmt.Printf("Error marshalling to JSON: %v\n", err)
					return
				}
				fmt.Println(string(out))
				return
			}

			if err := runQueueMonitor(client); err != nil {
				fmt.Printf("Queue monitor error: %v\n", err)
			}
		},
	}

	// Queue start command
	queueStartCmd := &cobra.Command{
		Use:   "start",
		Short: "Start queue processing",
		Long:  `Begin processing articles in the queue.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.StartQueue(); err != nil {
				fmt.Printf("Failed to start queue: %v\n", err)
				return
			}

			fmt.Println("Queue processing started")
		},
	}

	// Queue stop command
	queueStopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop queue processing",
		Long:  `Pause processing of the queue.`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.StopQueue(); err != nil {
				fmt.Printf("Failed to stop queue: %v\n", err)
				return
			}

			fmt.Println("Queue processing stopped")
		},
	}

	// Queue clear command
	queueClearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear the queue",
		Long:  `Remove all articles from the queue (does not stop active processing).`,
		Run: func(cmd *cobra.Command, args []string) {
			client := getNewDownlinkClient()

			if err := client.ClearQueue(); err != nil {
				fmt.Printf("Failed to clear queue: %v\n", err)
				return
			}

			fmt.Println("Queue cleared")
		},
	}

	cmd.AddCommand(queueStatusCmd, queueStartCmd, queueStopCmd, queueClearCmd)

	return cmd
}
