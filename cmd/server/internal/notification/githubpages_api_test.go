package notification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"downlink/pkg/models"
)

func TestParseGitHubRepoURL(t *testing.T) {
	tests := []struct {
		name      string
		repoURL   string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "https git URL",
			repoURL:   "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "https URL without git suffix",
			repoURL:   "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "non GitHub URL",
			repoURL: "https://example.com/owner/repo.git",
			wantErr: true,
		},
		{
			name:    "invalid path",
			repoURL: "https://github.com/owner",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotRepo, err := parseGitHubRepoURL(tt.repoURL)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if !strings.Contains(err.Error(), "invalid GitHub repo URL") {
					t.Fatalf("expected helpful GitHub repo URL error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseGitHubRepoURL() error = %v", err)
			}
			if gotOwner != tt.wantOwner || gotRepo != tt.wantRepo {
				t.Fatalf("parseGitHubRepoURL() = (%q, %q), want (%q, %q)", gotOwner, gotRepo, tt.wantOwner, tt.wantRepo)
			}
		})
	}
}

// branchExistsResponse is a minimal JSON body returned by the GitHub branches
// API; only needed in test mock servers.
type branchExistsResponse struct{}

func TestConfigureGitHubPagesSourceCreatesMissingSite(t *testing.T) {
	var postBody githubPagesSourceRequest
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/repos/owner/repo/branches/pages":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method on branches: %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(branchExistsResponse{})
		case "/repos/owner/repo/pages":
			switch r.Method {
			case http.MethodGet:
				w.WriteHeader(http.StatusNotFound)
			case http.MethodPost:
				if err := json.NewDecoder(r.Body).Decode(&postBody); err != nil {
					t.Fatalf("decode POST body: %v", err)
				}
				w.WriteHeader(http.StatusCreated)
			default:
				t.Fatalf("unexpected method %s", r.Method)
			}
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	withGitHubPagesAPIBaseURL(t, server.URL)

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:        "https://github.com/owner/repo.git",
		Branch:         "pages",
		Token:          "token",
		ConfigurePages: true,
	})

	if err := publisher.configureGitHubPagesSourceIfEnabled(); err != nil {
		t.Fatalf("configureGitHubPagesSourceIfEnabled() error = %v", err)
	}
	want := "GET /repos/owner/repo/branches/pages,GET /repos/owner/repo/pages,POST /repos/owner/repo/pages"
	if got := strings.Join(requests, ","); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
	if postBody.Source.Branch != "pages" || postBody.Source.Path != "/" {
		t.Fatalf("POST source = %+v, want branch pages path /", postBody.Source)
	}
}

// TestConfigureGitHubPagesSourceSeedsOrphanBranch verifies that a missing Pages
// branch is always seeded via the Git API as an orphan commit, regardless of
// whether the repository already has a default branch with commits. This avoids
// forking main's source code into the Pages branch.
func TestConfigureGitHubPagesSourceSeedsOrphanBranch(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/repos/owner/repo/branches/pages":
			w.WriteHeader(http.StatusNotFound)
		case "/repos/owner/repo/git/trees":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method on trees: %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(githubSHAResponse{SHA: "tree-sha"})
		case "/repos/owner/repo/git/commits":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method on commits: %s", r.Method)
			}
			var body githubCreateCommitRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode commit body: %v", err)
			}
			if body.Tree != "tree-sha" || len(body.Parents) != 0 {
				t.Fatalf("commit body = %+v, want orphan commit using tree-sha", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(githubSHAResponse{SHA: "commit-sha"})
		case "/repos/owner/repo/git/refs":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method on refs: %s", r.Method)
			}
			var body githubCreateRefRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode ref body: %v", err)
			}
			if body.Ref != "refs/heads/pages" || body.SHA != "commit-sha" {
				t.Fatalf("ref body = %+v, want refs/heads/pages at commit-sha", body)
			}
			w.WriteHeader(http.StatusCreated)
		case "/repos/owner/repo/pages":
			switch r.Method {
			case http.MethodGet:
				w.WriteHeader(http.StatusNotFound)
			case http.MethodPost:
				w.WriteHeader(http.StatusCreated)
			}
		default:
			t.Fatalf("unexpected path %s %s (repo/branches/main and git/refs must not be called)", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	withGitHubPagesAPIBaseURL(t, server.URL)

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:        "https://github.com/owner/repo.git",
		Branch:         "pages",
		Token:          "token",
		ConfigurePages: true,
	})

	if err := publisher.configureGitHubPagesSourceIfEnabled(); err != nil {
		t.Fatalf("configureGitHubPagesSourceIfEnabled() error = %v", err)
	}
	want := "GET /repos/owner/repo/branches/pages,POST /repos/owner/repo/git/trees,POST /repos/owner/repo/git/commits,POST /repos/owner/repo/git/refs,GET /repos/owner/repo/pages,POST /repos/owner/repo/pages"
	if got := strings.Join(requests, ","); got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestConfigureGitHubPagesSourceUpdatesMismatchedSite(t *testing.T) {
	var putBody githubPagesSourceRequest
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/branches/gh-pages":
			_ = json.NewEncoder(w).Encode(branchExistsResponse{})
			return
		case "/repos/owner/repo/pages":
			requests = append(requests, r.Method)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(githubPagesSite{
				Source: githubPagesSource{Branch: "main", Path: "/"},
			})
		case http.MethodPut:
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("decode PUT body: %v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", r.Method)
		}
	}))
	defer server.Close()
	withGitHubPagesAPIBaseURL(t, server.URL)

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:        "https://github.com/owner/repo",
		Branch:         "gh-pages",
		Token:          "token",
		ConfigurePages: true,
	})

	if err := publisher.configureGitHubPagesSourceIfEnabled(); err != nil {
		t.Fatalf("configureGitHubPagesSourceIfEnabled() error = %v", err)
	}
	if got, want := strings.Join(requests, ","), "GET,PUT"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
	if putBody.Source.Branch != "gh-pages" || putBody.Source.Path != "/" {
		t.Fatalf("PUT source = %+v, want branch gh-pages path /", putBody.Source)
	}
}

func TestConfigureGitHubPagesSourceNoopsForMatchingSite(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/repo/branches/pages":
			_ = json.NewEncoder(w).Encode(branchExistsResponse{})
			return
		case "/repos/owner/repo/pages":
			requests = append(requests, r.Method)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected write request %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(githubPagesSite{
			Source: githubPagesSource{Branch: "pages", Path: "/"},
		})
	}))
	defer server.Close()
	withGitHubPagesAPIBaseURL(t, server.URL)

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:        "https://github.com/owner/repo.git",
		Branch:         "pages",
		Token:          "token",
		ConfigurePages: true,
	})

	if err := publisher.configureGitHubPagesSourceIfEnabled(); err != nil {
		t.Fatalf("configureGitHubPagesSourceIfEnabled() error = %v", err)
	}
	if got, want := strings.Join(requests, ","), "GET"; got != want {
		t.Fatalf("requests = %s, want %s", got, want)
	}
}

func TestDeleteRemoteBranchIfExists(t *testing.T) {
	t.Run("deletes existing branch", func(t *testing.T) {
		deleted := false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/owner/repo/branches/gh-pages":
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(branchExistsResponse{})
			case "/repos/owner/repo/git/refs/heads/gh-pages":
				if r.Method != http.MethodDelete {
					t.Fatalf("expected DELETE, got %s", r.Method)
				}
				deleted = true
				w.WriteHeader(http.StatusNoContent)
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		withGitHubPagesAPIBaseURL(t, server.URL)

		publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
			RepoURL: "https://github.com/owner/repo.git",
			Branch:  "gh-pages",
			Token:   "token",
		})
		if err := publisher.deleteRemoteBranchIfExists("owner", "repo"); err != nil {
			t.Fatalf("deleteRemoteBranchIfExists() error = %v", err)
		}
		if !deleted {
			t.Fatalf("expected DELETE request to refs endpoint")
		}
	})

	t.Run("no-ops when branch missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/repos/owner/repo/branches/gh-pages":
				w.WriteHeader(http.StatusNotFound)
			default:
				t.Fatalf("unexpected path %s", r.URL.Path)
			}
		}))
		defer server.Close()
		withGitHubPagesAPIBaseURL(t, server.URL)

		publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
			RepoURL: "https://github.com/owner/repo.git",
			Branch:  "gh-pages",
			Token:   "token",
		})
		if err := publisher.deleteRemoteBranchIfExists("owner", "repo"); err != nil {
			t.Fatalf("deleteRemoteBranchIfExists() error = %v, want nil", err)
		}
	})
}

func TestConfigureGitHubPagesSourceAPIErrorOnlyWhenEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer server.Close()
	withGitHubPagesAPIBaseURL(t, server.URL)

	disabledPublisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL: "https://github.com/owner/repo.git",
		Branch:  "pages",
		Token:   "token",
	})
	if err := disabledPublisher.configureGitHubPagesSourceIfEnabled(); err != nil {
		t.Fatalf("disabled configure returned error = %v", err)
	}

	enabledPublisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:        "https://github.com/owner/repo.git",
		Branch:         "pages",
		Token:          "token",
		ConfigurePages: true,
	})
	if err := enabledPublisher.configureGitHubPagesSourceIfEnabled(); err == nil {
		t.Fatalf("enabled configure expected error")
	}
}

func withGitHubPagesAPIBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	old := githubPagesAPIBaseURL
	githubPagesAPIBaseURL = baseURL
	t.Cleanup(func() {
		githubPagesAPIBaseURL = old
	})
}
