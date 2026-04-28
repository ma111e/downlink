package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

var githubPagesAPIBaseURL = "https://api.github.com"

type githubPagesSite struct {
	Source githubPagesSource `json:"source"`
}

type githubPagesSource struct {
	Branch string `json:"branch"`
	Path   string `json:"path"`
}

type githubPagesSourceRequest struct {
	Source githubPagesSource `json:"source"`
}

func (p *GitHubPagesPublisher) configureGitHubPagesSource() error {
	owner, repo, err := parseGitHubRepoURL(p.cfg.RepoURL)
	if err != nil {
		return err
	}

	desired := githubPagesSource{Branch: p.cfg.Branch, Path: "/"}
	apiURL := fmt.Sprintf("%s/repos/%s/%s/pages", strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)

	var current githubPagesSite
	status, body, err := p.doGitHubPagesRequest(http.MethodGet, apiURL, nil, &current)
	if err != nil {
		return err
	}

	switch status {
	case http.StatusOK:
		if current.Source.Branch == desired.Branch && current.Source.Path == desired.Path {
			return nil
		}
		_, _, err = p.doGitHubPagesRequest(http.MethodPut, apiURL, githubPagesSourceRequest{Source: desired}, nil)
		return err
	case http.StatusNotFound:
		_, _, err = p.doGitHubPagesRequest(http.MethodPost, apiURL, githubPagesSourceRequest{Source: desired}, nil)
		return err
	default:
		return fmt.Errorf("get pages site returned %d: %s", status, body)
	}
}

func (p *GitHubPagesPublisher) configureGitHubPagesSourceIfEnabled() error {
	if !p.cfg.ConfigurePages {
		return nil
	}
	return p.configureGitHubPagesSource()
}

func (p *GitHubPagesPublisher) doGitHubPagesRequest(method, apiURL string, payload any, out any) (int, string, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return 0, "", fmt.Errorf("marshal request failed: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, apiURL, body)
	if err != nil {
		return 0, "", fmt.Errorf("build request failed: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if p.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("%s %s failed: %w", method, apiURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, "", fmt.Errorf("read response failed: %w", err)
	}
	responseBody := strings.TrimSpace(string(data))

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		return resp.StatusCode, responseBody, fmt.Errorf("%s pages site returned %d: %s", method, resp.StatusCode, responseBody)
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, responseBody, fmt.Errorf("decode response failed: %w", err)
		}
	}

	return resp.StatusCode, responseBody, nil
}

func parseGitHubRepoURL(repoURL string) (string, string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repo URL %q: %w", repoURL, err)
	}
	if u.Scheme != "https" || strings.ToLower(u.Hostname()) != "github.com" {
		return "", "", fmt.Errorf("invalid GitHub repo URL %q: expected an HTTPS github.com URL", repoURL)
	}

	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub repo URL %q: expected https://github.com/{owner}/{repo}.git", repoURL)
	}

	owner, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repo owner in %q: %w", repoURL, err)
	}
	repo, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid GitHub repo name in %q: %w", repoURL, err)
	}
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid GitHub repo URL %q: owner and repo are required", repoURL)
	}

	return owner, repo, nil
}
