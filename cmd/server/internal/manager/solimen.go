package manager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"downlink/pkg/models"
)

type solimenResp struct {
	State string `json:"state"`
	HTML  string `json:"html"`
}

func solimenScrape(addr, rawURL string, triggers models.HostTriggers) (solimenResp, error) {
	body, err := json.Marshal(struct {
		URL      string              `json:"url"`
		Triggers models.HostTriggers `json:"triggers"`
		Formats  []string            `json:"formats"`
	}{URL: rawURL, Triggers: triggers, Formats: []string{"html"}})
	if err != nil {
		return solimenResp{}, err
	}

	resp, err := http.Post(addr+"/scrape", "application/json", bytes.NewReader(body))
	if err != nil {
		return solimenResp{}, fmt.Errorf("solimen request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return solimenResp{}, fmt.Errorf("solimen returned status %d", resp.StatusCode)
	}

	var result solimenResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return solimenResp{}, fmt.Errorf("solimen response decode failed: %w", err)
	}
	return result, nil
}
