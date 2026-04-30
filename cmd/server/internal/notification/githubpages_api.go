package notification

import (
	"bytes"
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
// remote. If it is missing, it is seeded with a single orphan commit via the
// GitHub Git API. Creating an orphan commit (rather than branching from the
// default branch) guarantees the Pages branch contains no source code. This is
// a prerequisite for configuring GitHub Pages: the Pages create endpoint
// returns 422 if the source branch doesn't yet exist.
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
	}).Info("Branch missing on remote; seeding orphan branch for GitHub Pages")

	return p.seedEmptyRepo(owner, repo, p.cfg.Branch)
}

// deleteRemoteBranchIfExists removes the configured branch from the remote if
// it exists. Used by --reinit-gh-pages to wipe the branch before recreating it.
func (p *GitHubPagesPublisher) deleteRemoteBranchIfExists(owner, repo string) error {
	branchURL := fmt.Sprintf("%s/repos/%s/%s/branches/%s",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo, url.PathEscape(p.cfg.Branch))
	status, _, err := p.doGitHubPagesRequest(http.MethodGet, branchURL, nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}

	refURL := fmt.Sprintf("%s/repos/%s/%s/git/refs/heads/%s",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo, url.PathEscape(p.cfg.Branch))
	status, body, err := p.doGitHubPagesRequest(http.MethodDelete, refURL, nil, nil)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete branch returned %d: %s", status, body)
	}
	log.WithFields(log.Fields{
		"owner":  owner,
		"repo":   repo,
		"branch": p.cfg.Branch,
	}).Info("Deleted remote branch for reinitialisation")
	return nil
}

type githubCreateTreeRequest struct {
	Tree []githubTreeEntry `json:"tree"`
}

type githubTreeEntry struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"`
	Type    string `json:"type"`
	Content string `json:"content"`
}

type githubSHAResponse struct {
	SHA string `json:"sha"`
}

type githubCreateCommitRequest struct {
	Message string   `json:"message"`
	Tree    string   `json:"tree"`
	Parents []string `json:"parents"`
}

type githubCreateRefRequest struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

func (p *GitHubPagesPublisher) seedEmptyRepo(owner, repo, branch string) error {
	treeURL := fmt.Sprintf("%s/repos/%s/%s/git/trees",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	treePayload := githubCreateTreeRequest{
		Tree: []githubTreeEntry{{
			Path:    ".gitkeep",
			Mode:    "100644",
			Type:    "blob",
			Content: "",
		}},
	}
	var tree githubSHAResponse
	status, body, err := p.doGitHubPagesRequest(http.MethodPost, treeURL, treePayload, &tree)
	if err != nil {
		return err
	}
	if status != http.StatusCreated || tree.SHA == "" {
		return fmt.Errorf("create seed tree returned %d: %s", status, body)
	}

	commitURL := fmt.Sprintf("%s/repos/%s/%s/git/commits",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	commitPayload := githubCreateCommitRequest{
		Message: "Initialize " + branch + " for GitHub Pages",
		Tree:    tree.SHA,
		Parents: []string{},
	}
	var commit githubSHAResponse
	status, body, err = p.doGitHubPagesRequest(http.MethodPost, commitURL, commitPayload, &commit)
	if err != nil {
		return err
	}
	if status != http.StatusCreated || commit.SHA == "" {
		return fmt.Errorf("create seed commit returned %d: %s", status, body)
	}

	refURL := fmt.Sprintf("%s/repos/%s/%s/git/refs",
		strings.TrimRight(githubPagesAPIBaseURL, "/"), owner, repo)
	refPayload := githubCreateRefRequest{
		Ref: "refs/heads/" + branch,
		SHA: commit.SHA,
	}
	status, body, err = p.doGitHubPagesRequest(http.MethodPost, refURL, refPayload, nil)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("create seed ref returned %d: %s", status, body)
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
