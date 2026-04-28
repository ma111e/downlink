package main

import (
	"embed"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"downlink/pkg/downlinkclient"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	var address string
	var port int

	// Define the root command
	var rootCmd = &cobra.Command{
		Use:   "ui",
		Short: "DOWNLINK UI",
		Run: func(cmd *cobra.Command, args []string) {
			// Create an instance of the app structure
			var opts []grpc.DialOption

			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
			opts = append(opts, grpc.WithConnectParams(grpc.ConnectParams{
				MinConnectTimeout: 10 * time.Second,
			}))
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

			downlinkCLient := downlinkclient.NewDownlinkClient(conn)

			err = wails.Run(&options.App{
				Title:  "downlink",
				Width:  1024,
				Height: 768,
				AssetServer: &assetserver.Options{
					Assets: assets,
				},
				BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
				OnStartup:        downlinkCLient.Entry,
				OnShutdown:       downlinkCLient.Shutdown,
				Bind: []interface{}{
					downlinkCLient,
				},
				LogLevel: logger.INFO,
			})

			if err != nil {
				log.WithFields(log.Fields{
					"err": err,
				}).Fatalln("Failed to start wails")
			}
		},
	}

	// Add flags for address and port
	rootCmd.Flags().StringVarP(&address, "address", "a", "localhost", "Host to run the RPC server on")
	rootCmd.Flags().IntVarP(&port, "port", "p", 50051, "Port to run the RPC server on")

	// Execute the command
	if err := rootCmd.Execute(); err != nil {
		log.WithFields(log.Fields{
			"err": err,
		}).Fatalln("Failed to start client")
	}
}
