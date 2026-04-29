package notification

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"downlink/pkg/models"

	log "github.com/sirupsen/logrus"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

// GitHubPagesPublisher publishes digest HTML files to a GitHub Pages repository.
//
// Each publish writes the new digest HTML and updates a manifest.json that
// lists every published digest. Both the per-digest switcher and the index
// page are static shells that read manifest.json in the browser, so old HTML
// files are never rewritten on subsequent publishes.
type GitHubPagesPublisher struct {
	cfg models.GitHubPagesNotificationConfig
}

// NewGitHubPagesPublisher creates a new GitHubPagesPublisher.
func NewGitHubPagesPublisher(cfg models.GitHubPagesNotificationConfig) *GitHubPagesPublisher {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.CommitAuthor == "" {
		cfg.CommitAuthor = "downlink-bot"
	}
	if cfg.CommitEmail == "" {
		cfg.CommitEmail = "downlink-bot@users.noreply.github.com"
	}
	if cfg.CloneDir == "" {
		cfg.CloneDir = filepath.Join(os.TempDir(), "downlink-ghpages")
	}
	return &GitHubPagesPublisher{cfg: cfg}
}

// SendDigest renders the digest HTML, updates manifest.json, and pushes the
// result to the GitHub Pages repo. Old digest pages are not touched.
func (p *GitHubPagesPublisher) SendDigest(digest models.Digest) error {
	log.WithField("digestId", digest.Id).Info("Publishing digest to GitHub Pages")

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	if err := p.configureGitHubPagesSourceIfEnabled(); err != nil {
		return fmt.Errorf("github pages: failed to configure GitHub Pages source: %w", err)
	}

	repo, err := p.ensureRepo(auth)
	if err != nil {
		return fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("github pages: failed to get worktree: %w", err)
	}

	digestRelPath, err := p.renderAndStage(wt, digest)
	if err != nil {
		return err
	}

	manifestRelPath := ManifestFilename
	if p.cfg.OutputDir != "" {
		manifestRelPath = filepath.Join(p.cfg.OutputDir, ManifestFilename)
	}
	manifestAbsPath := filepath.Join(p.cfg.CloneDir, manifestRelPath)

	manifest, err := LoadManifest(manifestAbsPath)
	if err != nil {
		return fmt.Errorf("github pages: load manifest: %w", err)
	}
	manifest.Upsert(ManifestEntryFromDigest(digest))
	if err := manifest.Write(manifestAbsPath); err != nil {
		return fmt.Errorf("github pages: write manifest: %w", err)
	}
	if _, err := wt.Add(manifestRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage manifest: %w", err)
	}

	if err := p.ensureIndex(wt); err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("Publish digest %s (%s)", digest.Id[:8], digest.CreatedAt.UTC().Format("2006-01-02"))
	_, err = wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("github pages: failed to commit: %w", err)
	}

	pushOpts := &gogit.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	}
	if err := repo.Push(pushOpts); err != nil {
		if isNonFastForward(err) {
			log.Warn("GitHub Pages push rejected (non-fast-forward); pulling and retrying")
			pullErr := wt.Pull(&gogit.PullOptions{
				RemoteName:    "origin",
				ReferenceName: plumbing.NewBranchReferenceName(p.cfg.Branch),
				Auth:          auth,
				Force:         true,
			})
			if pullErr != nil && pullErr != gogit.NoErrAlreadyUpToDate {
				return fmt.Errorf("github pages: rebase pull failed: %w", pullErr)
			}
			if retryErr := repo.Push(pushOpts); retryErr != nil {
				return fmt.Errorf("github pages: push retry failed: %w", retryErr)
			}
		} else {
			return fmt.Errorf("github pages: push failed: %w", err)
		}
	}

	var pageURL string
	if p.cfg.BaseURL != "" {
		base := strings.TrimRight(p.cfg.BaseURL, "/")
		pageURL = base + "/" + digestRelPath
		log.WithField("url", pageURL).Info("Digest published to GitHub Pages")
	} else {
		log.WithField("file", digestRelPath).Info("Digest published to GitHub Pages")
	}

	if p.cfg.DiscordWebhookURL != "" {
		msg := "📰 New digest published to GitHub Pages"
		if pageURL != "" {
			msg += ": " + pageURL
		}
		if err := SendDiscordMessage(p.cfg.DiscordWebhookURL, msg); err != nil {
			log.WithError(err).Warn("Failed to send GitHub Pages publish notification to Discord")
		}
	}

	return nil
}

// ensureRepo clones the remote repo if the local clone dir is absent, or pulls
// the latest changes if it already exists.
func (p *GitHubPagesPublisher) ensureRepo(auth *githttp.BasicAuth) (*gogit.Repository, error) {
	branchRef := plumbing.NewBranchReferenceName(p.cfg.Branch)

	if _, err := os.Stat(filepath.Join(p.cfg.CloneDir, ".git")); os.IsNotExist(err) {
		log.WithFields(log.Fields{
			"repoURL":  p.cfg.RepoURL,
			"cloneDir": p.cfg.CloneDir,
			"branch":   p.cfg.Branch,
		}).Info("Cloning GitHub Pages repository")

		repo, err := gogit.PlainClone(p.cfg.CloneDir, false, &gogit.CloneOptions{
			URL:           p.cfg.RepoURL,
			Auth:          auth,
			ReferenceName: branchRef,
			SingleBranch:  true,
			Depth:         1,
		})
		if err != nil {
			return nil, fmt.Errorf("clone failed: %w", err)
		}
		return repo, nil
	}

	repo, err := gogit.PlainOpen(p.cfg.CloneDir)
	if err != nil {
		return nil, fmt.Errorf("open existing clone failed: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("get worktree failed: %w", err)
	}

	pullErr := wt.Pull(&gogit.PullOptions{
		RemoteName:    "origin",
		ReferenceName: branchRef,
		Auth:          auth,
		Force:         true,
	})
	if pullErr != nil && pullErr != gogit.NoErrAlreadyUpToDate {
		return nil, fmt.Errorf("pull failed: %w", pullErr)
	}

	return repo, nil
}

// ensureIndex writes index.html only when missing or when its rendered bytes
// differ from disk content. The index is a static shell that pulls its list
// from manifest.json at runtime, so it rarely changes.
func (p *GitHubPagesPublisher) ensureIndex(wt *gogit.Worktree) error {
	indexRelPath := "index.html"
	if p.cfg.OutputDir != "" {
		indexRelPath = filepath.Join(p.cfg.OutputDir, "index.html")
	}
	indexAbsPath := filepath.Join(p.cfg.CloneDir, indexRelPath)

	indexBytes, err := RenderDigestIndex()
	if err != nil {
		return fmt.Errorf("github pages: failed to build index: %w", err)
	}

	existing, readErr := os.ReadFile(indexAbsPath)
	if readErr == nil && bytes.Equal(existing, indexBytes) {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(indexAbsPath), 0755); err != nil {
		return fmt.Errorf("github pages: failed to create index dir: %w", err)
	}
	if err := os.WriteFile(indexAbsPath, indexBytes, 0644); err != nil {
		return fmt.Errorf("github pages: failed to write index HTML: %w", err)
	}
	if _, err := wt.Add(indexRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage index file: %w", err)
	}
	return nil
}

func isNonFastForward(err error) bool {
	return err != nil && strings.Contains(err.Error(), "non-fast-forward")
}

// renderAndStage renders a digest, writes it to the publisher's output dir,
// and stages it in the worktree. It returns the staged file's repo-relative
// path.
func (p *GitHubPagesPublisher) renderAndStage(wt *gogit.Worktree, digest models.Digest) (string, error) {
	htmlBytes, err := RenderDigestHTML(digest, "dark")
	if err != nil {
		return "", fmt.Errorf("github pages: failed to render digest HTML: %w", err)
	}

	digestFilename := DigestHTMLFilename(digest)
	digestRelPath := digestFilename
	if p.cfg.OutputDir != "" {
		digestRelPath = filepath.Join(p.cfg.OutputDir, digestFilename)
	}
	digestAbsPath := filepath.Join(p.cfg.CloneDir, digestRelPath)

	if p.cfg.OutputDir != "" {
		if err := os.MkdirAll(filepath.Dir(digestAbsPath), 0755); err != nil {
			return "", fmt.Errorf("github pages: failed to create output dir: %w", err)
		}
	}

	if err := os.WriteFile(digestAbsPath, htmlBytes, 0644); err != nil {
		return "", fmt.Errorf("github pages: failed to write digest HTML: %w", err)
	}

	if _, err := wt.Add(digestRelPath); err != nil {
		return "", fmt.Errorf("github pages: failed to stage digest file: %w", err)
	}
	return digestRelPath, nil
}
