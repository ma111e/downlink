package envoverride

import (
	"fmt"
	"os"
	"strings"
)

// Apply parses KEY=VALUE entries and applies them to the current process
// environment in order.
func Apply(entries []string) error {
	for _, entry := range entries {
		key, value, err := Parse(entry)
		if err != nil {
			return err
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	return nil
}

// Parse validates and splits one KEY=VALUE environment override.
func Parse(entry string) (string, string, error) {
	key, value, ok := strings.Cut(entry, "=")
	if !ok {
		return "", "", fmt.Errorf("invalid --env %q: expected KEY=VALUE", entry)
	}
	if key == "" {
		return "", "", fmt.Errorf("invalid --env %q: key cannot be empty", entry)
	}
	if !validName(key) {
		return "", "", fmt.Errorf("invalid --env %q: %q is not a valid environment variable name", entry, key)
	}
	return key, value, nil
}

func validName(name string) bool {
	for i, r := range name {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}
