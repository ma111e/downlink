package notification

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	log "github.com/sirupsen/logrus"
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

	if err := p.ensureRemoteBranchExists(owner, repo); err != nil {
		return fmt.Errorf("ensure branch %q exists: %w", p.cfg.Branch, err)
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

// ensureRemoteBranchExists makes sure the configured branch exists on the
// remote. If it is missing, it is created — either branched off the repo's
// default branch when there is one, or as the repo's first commit when the
// repo is empty. This is a prerequisite for configuring GitHub Pages: the
// Pages create endpoint returns 422 if the source branch doesn't yet exist.
func (p *GitHubPagesPublisher) ensureRemoteBranchExists(owner, repo string) error {
	branchURL := fmt.Sprintf("%s/repos/%s/%s/branches/%s",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo, url.PathEscape(p.cfg.Branch))
	status, body, err := p.doGitHubPagesRequest(http.MethodGet, branchURL, nil, nil)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK:
		return nil
	case http.StatusNotFound:
		// fall through to creation
	default:
		return fmt.Errorf("get branch returned %d: %s", status, body)
	}

	log.WithFields(log.Fields{
		"owner":  owner,
		"repo":   repo,
		"branch": p.cfg.Branch,
	}).Info("Branch missing on remote; creating it for GitHub Pages")

	// Try to base the new branch off the repo's default branch.
	defaultBranch, defaultSHA, err := p.lookupDefaultBranchHead(owner, repo)
	if err != nil {
		return err
	}
	if defaultSHA != "" {
		return p.createBranchRef(owner, repo, p.cfg.Branch, defaultSHA, defaultBranch)
	}

	// Repo is empty (no default branch HEAD). Seed it by creating an initial
	// file directly on the target branch via the Contents API.
	return p.seedEmptyRepo(owner, repo, p.cfg.Branch)
}

type githubRepoInfo struct {
	DefaultBranch string `json:"default_branch"`
}

type githubBranchInfo struct {
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

func (p *GitHubPagesPublisher) lookupDefaultBranchHead(owner, repo string) (string, string, error) {
	repoURL := fmt.Sprintf("%s/repos/%s/%s",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	var info githubRepoInfo
	status, body, err := p.doGitHubPagesRequest(http.MethodGet, repoURL, nil, &info)
	if err != nil {
		return "", "", err
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("get repo returned %d: %s", status, body)
	}
	if info.DefaultBranch == "" {
		return "", "", nil
	}

	branchURL := fmt.Sprintf("%s/repos/%s/%s/branches/%s",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo, url.PathEscape(info.DefaultBranch))
	var branch githubBranchInfo
	status, body, err = p.doGitHubPagesRequest(http.MethodGet, branchURL, nil, &branch)
	if err != nil {
		return "", "", err
	}
	if status == http.StatusNotFound {
		// Default branch advertised but no commits yet — treat as empty.
		return info.DefaultBranch, "", nil
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("get default branch returned %d: %s", status, body)
	}
	return info.DefaultBranch, branch.Commit.SHA, nil
}

type githubCreateRefRequest struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

func (p *GitHubPagesPublisher) createBranchRef(owner, repo, branch, sha, baseBranch string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/git/refs",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	payload := githubCreateRefRequest{
		Ref: "refs/heads/" + branch,
		SHA: sha,
	}
	status, body, err := p.doGitHubPagesRequest(http.MethodPost, apiURL, payload, nil)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("create ref returned %d: %s", status, body)
	}
	log.WithFields(log.Fields{
		"branch":     branch,
		"baseBranch": baseBranch,
		"baseSHA":    sha,
	}).Info("Created remote branch from default branch")
	return nil
}

type githubPutContentsRequest struct {
	Message string `json:"message"`
	Content string `json:"content"`
	Branch  string `json:"branch"`
}

func (p *GitHubPagesPublisher) seedEmptyRepo(owner, repo, branch string) error {
	apiURL := fmt.Sprintf("%s/repos/%s/%s/contents/.gitkeep",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	payload := githubPutContentsRequest{
		Message: "Initialize " + branch + " for GitHub Pages",
		Content: base64.StdEncoding.EncodeToString([]byte("")),
		Branch:  branch,
	}
	status, body, err := p.doGitHubPagesRequest(http.MethodPut, apiURL, payload, nil)
	if err != nil {
		return err
	}
	if status != http.StatusCreated && status != http.StatusOK {
		return fmt.Errorf("seed empty repo returned %d: %s", status, body)
	}
	log.WithField("branch", branch).Info("Seeded empty repo with initial commit on branch")
	return nil
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
