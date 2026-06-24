package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/ma111e/downlink/pkg/models"
	"github.com/ma111e/downlink/pkg/trace"
)

type solimenResp struct {
	State string `json:"state"`
	HTML  string `json:"html"`
}

func solimenScrape(articleID, addr, rawURL string, triggers models.HostTriggers) (solimenResp, error) {
	body, err := json.Marshal(struct {
		URL      string              `json:"url"`
		Triggers models.HostTriggers `json:"triggers"`
		Formats  []string            `json:"formats"`
	}{URL: rawURL, Triggers: triggers, Formats: []string{"html"}})
	if err != nil {
		return solimenResp{}, err
	}

	if trace.Enabled() {
		trace.SolimenRequest(articleID, rawURL, body)
	}

	resp, err := http.Post(addr+"/scrape", "application/json", bytes.NewReader(body))
	if err != nil {
		return solimenResp{}, fmt.Errorf("solimen request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Solimen returns its real failure reason in the response body
		// ({"error": "chromium scraper: ..."}). Surface it instead of a bare status.
		if msg := readSolimenError(resp.Body); msg != "" {
			return solimenResp{}, fmt.Errorf("solimen returned status %d: %s", resp.StatusCode, msg)
		}
		return solimenResp{}, fmt.Errorf("solimen returned status %d", resp.StatusCode)
	}

	var result solimenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return solimenResp{}, fmt.Errorf("solimen response decode failed: %w", err)
	}
	return result, nil
}

// readSolimenError extracts a human-readable reason from a Solimen error response body.
// It prefers the JSON {"error": ...} field and falls back to the trimmed raw body. The
// read is bounded since this runs on the failure path.
func readSolimenError(r io.Reader) string {
	raw, err := io.ReadAll(io.LimitReader(r, 8<<10))
	if err != nil || len(raw) == 0 {
		return ""
	}
	var payload struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.Error != "" {
		return payload.Error
	}
	return strings.TrimSpace(string(raw))
}
