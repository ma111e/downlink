package main

import (
	"downlink/pkg/downlinkclient"
	"downlink/pkg/envoverride"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	// Global flags
	address    string
	port       int
	jsonOutput bool
	envVars    []string
)

func main() {
	cobra.EnableTraverseRunHooks = true

	rootCmd := &cobra.Command{
		Use:   "downlink-cli",
		Short: "DOWNLINK CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return envoverride.Apply(envVars)
		},
	}

	// Set up global flags
	rootCmd.PersistentFlags().StringVar(&address, "address", "localhost", "gRPC server address")
	rootCmd.PersistentFlags().IntVar(&port, "port", 50051, "gRPC server port")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().StringArrayVar(&envVars, "env", nil, "Set an environment variable override (KEY=VALUE); may be repeated")

	// Add all command groups
	rootCmd.AddCommand(createArticleCommands())
	rootCmd.AddCommand(createFeedCommands())
	rootCmd.AddCommand(createAnalysisCommands())
	rootCmd.AddCommand(createLLMCommands())
	rootCmd.AddCommand(createDigestCommands())
	rootCmd.AddCommand(createConfigCommands())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// getNewDownlinkClient creates a connection to the gRPC server and returns an DownlinkClient instance
func getNewDownlinkClient() *downlinkclient.DownlinkClient {
	var opts []grpc.DialOption

	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(fmt.Sprintf("%s:%d", address, port), opts...)
	if err != nil {
		log.WithFields(log.Fields{
			"address": address,
			"port":    port,
			"err":     err,
		}).Fatalln("Failed to connect to gRPC server")
	}

	log.WithFields(log.Fields{
		"address": address,
		"port":    port,
	}).Info("Connected to gRPC server")

	client := downlinkclient.NewDownlinkClient(conn)

	return client
}
