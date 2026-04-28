package scrapers

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const (
	lightpandaCDPHost    = "localhost"
	lightpandaCDPPort    = "9222"
	lightpandaDockerName = "lightpanda"
	lightpandaDockerCmd  = "docker run -d --name lightpanda -p localhost:9222:9222 lightpanda/browser"
)

// CheckLightpanda checks if Lightpanda is reachable on port 9222.
// If not, it either prompts the user interactively or auto-starts the container.
// Set autoStart to true to skip the interactive prompt.
func CheckLightpanda(autoStart bool) error {
	if isLightpandaReachable() {
		log.Info("Lightpanda is running and reachable on port 9222")
		return nil
	}

	log.Warn("Lightpanda is not reachable on port 9222")

	if autoStart {
		log.Info("Auto-starting Lightpanda Docker container...")
		return startLightpandaDocker()
	}

	// Interactive prompt
	fmt.Println()
	fmt.Println("⚠  Lightpanda browser is not running.")
	fmt.Println("   Dynamic scraping (Playwright) requires Lightpanda in Docker on port 9222.")
	fmt.Println()
	fmt.Println("   Start it with:")
	fmt.Printf("   %s\n", lightpandaDockerCmd)
	fmt.Println()
	fmt.Print("Would you like to start it now? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" {
		return startLightpandaDocker()
	}

	log.Warn("Lightpanda not started — dynamic scraping will be unavailable")
	return nil
}

// isLightpandaReachable checks if something is listening on the Lightpanda CDP port.
func isLightpandaReachable() bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(lightpandaCDPHost, lightpandaCDPPort), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startLightpandaDocker attempts to start the Lightpanda Docker container.
func startLightpandaDocker() error {
	// First try to start an existing stopped container
	startCmd := exec.Command("docker", "start", lightpandaDockerName)
	if err := startCmd.Run(); err == nil {
		// Container existed and was started
		log.Info("Started existing Lightpanda Docker container")
		return waitForLightpanda()
	}

	// Container doesn't exist — create and run it
	log.Info("Creating new Lightpanda Docker container...")
	runCmd := exec.Command("docker", "run", "-d", "--name", lightpandaDockerName, "-p", "9222:9222", "lightpanda/browser")
	output, err := runCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Lightpanda Docker container: %w\nOutput: %s", err, string(output))
	}

	log.Info("Lightpanda Docker container created")
	return waitForLightpanda()
}

// waitForLightpanda waits for Lightpanda to become reachable after starting.
func waitForLightpanda() error {
	log.Info("Waiting for Lightpanda to become ready...")
	for i := 0; i < 15; i++ {
		if isLightpandaReachable() {
			log.Info("Lightpanda is ready on port 9222")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("Lightpanda did not become reachable within 15 seconds")
}
