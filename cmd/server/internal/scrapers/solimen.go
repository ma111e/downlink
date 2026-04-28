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
	solimenHost        = "localhost"
	solimenPort        = "5011"
	solimenDockerName  = "solimen"
	solimenDockerImage = "ghcr.io/ma111e/solimen:latest"
)

// CheckSolimen checks if the Solimen service is reachable on port 5011.
// If not, it either prompts the user interactively or auto-starts the container.
func CheckSolimen(autoStart bool) error {
	if isSolimenReachable() {
		log.Info("Solimen is running and reachable on port 5011")
		return nil
	}

	log.Warn("Solimen is not reachable on port 5011")

	if autoStart {
		log.Info("Auto-starting Solimen Docker container...")
		return startSolimenDocker()
	}

	// Interactive prompt
	fmt.Println()
	fmt.Println("⚠  Solimen browser service is not running.")
	fmt.Println("   Full browser scraping requires Solimen in Docker on port 5011.")
	fmt.Println()
	fmt.Println("   Start it with:")
	fmt.Printf("   docker run -d --name %s -p %s:%s %s\n", solimenDockerName, solimenPort, solimenPort, solimenDockerImage)
	fmt.Println()
	fmt.Print("Would you like to start it now? [Y/n] ")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read user input: %w", err)
	}

	answer = strings.TrimSpace(strings.ToLower(answer))
	if answer == "" || answer == "y" || answer == "yes" {
		return startSolimenDocker()
	}

	log.Warn("Solimen not started — full_browser scraping will be unavailable")
	return nil
}

// isSolimenReachable checks if something is listening on the Solimen port.
func isSolimenReachable() bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(solimenHost, solimenPort), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// startSolimenDocker attempts to start the Solimen Docker container.
func startSolimenDocker() error {
	// First try to start an existing stopped container
	startCmd := exec.Command("docker", "start", solimenDockerName)
	if err := startCmd.Run(); err == nil {
		log.Info("Started existing Solimen Docker container")
		return waitForSolimen()
	}

	// Container doesn't exist — pull and run from hosted image
	log.Info("Creating new Solimen Docker container...")
	runCmd := exec.Command("docker", "run", "-d", "--name", solimenDockerName, "-p", solimenPort+":"+solimenPort, solimenDockerImage)
	output, err := runCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Solimen Docker container: %w\nOutput: %s", err, string(output))
	}

	log.Info("Solimen Docker container started")
	return waitForSolimen()
}

// waitForSolimen waits for Solimen to become reachable after starting.
func waitForSolimen() error {
	log.Info("Waiting for Solimen to become ready...")
	for i := 0; i < 30; i++ {
		if isSolimenReachable() {
			log.Info("Solimen is ready on port 5011")
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	return fmt.Errorf("Solimen did not become reachable within 30 seconds")
}
