package notification

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ma111e/downlink/pkg/models"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

// PublishProgress receives coarse step updates during publish/republish so a
// CLI front-end can render live progress. A nil PublishProgress disables
// reporting (used by the automatic server publish path).
type PublishProgress interface {
	Start(step, label string)                   // begin a step (spinner row)
	Update(step, label string)                  // relabel an in-flight step
	Complete(step string, ok bool, note string) // finish a step (✓ / ✗)
}

// GitHubPagesPublisher publishes digest HTML files to a GitHub Pages repository.
//
// Each publish writes the new digest HTML and updates a manifest.json that
// lists every published digest. Both the per-digest switcher and the index
// page are static shells that read manifest.json in the browser, so old HTML
// files are never rewritten on subsequent publishes.
type GitHubPagesPublisher struct {
	cfg         models.GitHubPagesNotificationConfig
	progress    PublishProgress
	listDigests DigestLister
}

// DigestLister returns up to limit newest digests with full payload (provider
// results + analyses). It lets the publisher build the RSS/Atom feeds without
// depending on the store: callers that have DB or client access supply it via
// SetDigestLister. A nil lister disables feed generation.
type DigestLister func(limit int) ([]models.Digest, error)

// FeedDigestLimit caps how many recent digests appear in the RSS/Atom feeds.
const FeedDigestLimit = 7

// SetDigestLister attaches the lister used to fetch recent digests when building
// the RSS/Atom feeds on each push. Passing nil disables feed generation.
func (p *GitHubPagesPublisher) SetDigestLister(fn DigestLister) {
	p.listDigests = fn
}

// SetProgress attaches a PublishProgress sink so callers can render live step
// progress for publish/republish operations. Passing nil disables reporting.
func (p *GitHubPagesPublisher) SetProgress(pr PublishProgress) {
	p.progress = pr
}

func (p *GitHubPagesPublisher) pStart(step, label string) {
	if p.progress != nil {
		p.progress.Start(step, label)
	}
}

func (p *GitHubPagesPublisher) pUpdate(step, label string) {
	if p.progress != nil {
		p.progress.Update(step, label)
	}
}

func (p *GitHubPagesPublisher) pComplete(step string, ok bool, note string) {
	if p.progress != nil {
		p.progress.Complete(step, ok, note)
	}
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
	_, err := p.sendDigest(digest)
	return err
}

// sendDigest performs the work of SendDigest and additionally returns the SHA
// of the pushed commit so callers (e.g. Republish) can wait for the
// corresponding GitHub Pages build to deploy.
func (p *GitHubPagesPublisher) sendDigest(digest models.Digest) (string, error) {
	log.WithField("digestId", digest.Id).Info("Publishing digest to GitHub Pages")

	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return "", fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	repo, err := p.ensureRepo(auth)
	if err != nil {
		return "", fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("github pages: failed to get worktree: %w", err)
	}

	digestRelPath, err := p.renderAndStage(wt, digest, outputDir, "dark")
	if err != nil {
		return "", err
	}

	if err := p.renderAndStageSwipe(wt, digest, outputDir); err != nil {
		return "", err
	}

	if err := p.writeAndStageManifest(wt, digest, outputDir); err != nil {
		return "", err
	}
	if feedDigests, err := p.recentFeedDigests(digest, FeedDigestLimit); err != nil {
		log.WithError(err).Warn("github pages: skipping feed update — failed to list recent digests")
	} else if err := p.writeAndStageFeeds(wt, outputDir, feedDigests); err != nil {
		return "", err
	}
	if err := p.ensureIndex(wt, outputDir); err != nil {
		return "", err
	}

	commitMsg := fmt.Sprintf("Publish digest %s (%s)", digest.Id, digest.CreatedAt.UTC().Format("2006-01-02"))
	commitHash, err := wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("github pages: failed to commit: %w", err)
	}

	if err := p.pushWithRetry(repo, wt, auth); err != nil {
		return "", err
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

	return commitHash.String(), nil
}

// RemoveDigest removes the digest identified by title from the archive.
// It resolves the title to a filename via the manifest, deletes both the
// digest and swipe HTML files, updates manifest.json, commits, and pushes.
// It returns the SHA of the pushed commit so callers can wait for the
// corresponding GitHub Pages build to deploy (see WaitForPagesBuild).
func (p *GitHubPagesPublisher) RemoveDigest(title string) (string, error) {
	log.WithField("title", title).Info("Removing digest from GitHub Pages")

	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return "", fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	repo, err := p.ensureRepo(auth)
	if err != nil {
		return "", fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("github pages: failed to get worktree: %w", err)
	}

	// Resolve title → filename via the manifest.
	manifestRelPath := filepath.Join(outputDir, ManifestFilename)
	manifestAbsPath := filepath.Join(p.cfg.CloneDir, manifestRelPath)
	manifest, err := LoadManifest(manifestAbsPath)
	if err != nil {
		return "", fmt.Errorf("github pages: load manifest: %w", err)
	}
	entry, ok := manifest.FindByTitle(title)
	if !ok {
		return "", fmt.Errorf("github pages: no digest with title %q found in manifest", title)
	}
	digestFilename := entry.Filename

	// Remove digest HTML.
	digestRelPath := filepath.Join(outputDir, digestFilename)
	if fileExists(filepath.Join(p.cfg.CloneDir, digestRelPath)) {
		if _, err := wt.Remove(digestRelPath); err != nil {
			return "", fmt.Errorf("github pages: failed to stage digest removal: %w", err)
		}
	}

	// Remove swipe HTML (same timestamp, different prefix).
	swipeFilename := strings.Replace(digestFilename, "downlink-digest-", "downlink-swipe-", 1)
	swipeRelPath := filepath.Join(outputDir, swipeFilename)
	if fileExists(filepath.Join(p.cfg.CloneDir, swipeRelPath)) {
		if _, err := wt.Remove(swipeRelPath); err != nil {
			return "", fmt.Errorf("github pages: failed to stage swipe removal: %w", err)
		}
	}

	// Drop the entry from the manifest and re-stage it.
	manifest.Remove(digestFilename)
	if err := manifest.Write(manifestAbsPath); err != nil {
		return "", fmt.Errorf("github pages: write manifest: %w", err)
	}
	if _, err := wt.Add(manifestRelPath); err != nil {
		return "", fmt.Errorf("github pages: failed to stage manifest: %w", err)
	}

	commitMsg := fmt.Sprintf("Remove digest %q", title)
	commitHash, err := wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", fmt.Errorf("github pages: failed to commit: %w", err)
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
				return "", fmt.Errorf("github pages: rebase pull failed: %w", pullErr)
			}
			if retryErr := repo.Push(pushOpts); retryErr != nil {
				return "", fmt.Errorf("github pages: push retry failed: %w", retryErr)
			}
		} else {
			return "", fmt.Errorf("github pages: push failed: %w", err)
		}
	}

	log.WithField("title", title).Info("Digest removed from GitHub Pages")
	return commitHash.String(), nil
}

// Republish removes a single digest from the archive and re-publishes it with
// the current templates (equivalent to remove followed by add). The removal is
// always awaited before the re-add is pushed: GitHub cancels an in-flight Pages
// build when a newer commit lands, so pushing the re-add immediately would skip
// deploying the removal. The final deploy is only awaited when wait is true.
func (p *GitHubPagesPublisher) Republish(digest models.Digest, wait bool) error {
	p.pStart("remove", "Removing from archive")
	removeSHA, err := p.RemoveDigest(digest.Title)
	if err != nil {
		p.pComplete("remove", false, "remove failed")
		return fmt.Errorf("remove digest: %w", err)
	}
	p.pComplete("remove", true, "removed "+shortSHA(removeSHA))

	// Best-effort: a wait failure must not leave the digest removed-but-not-re-added.
	if err := p.waitForDeploy(removeSHA, "deploy-remove", "Waiting for removal to deploy", true); err != nil {
		log.WithError(err).Warn("waiting for removal to deploy")
	}

	p.pStart("republish", "Re-rendering & pushing")
	reAddSHA, err := p.sendDigest(digest)
	if err != nil {
		p.pComplete("republish", false, "publish failed")
		return err
	}
	p.pComplete("republish", true, "pushed "+shortSHA(reAddSHA))

	return p.waitForDeploy(reAddSHA, "deploy", "Waiting for GitHub Pages deploy", wait)
}

// ManifestTitles clones (or updates) the repo and returns the list of digest
// titles from the manifest, newest-first. Returns an empty slice when the
// manifest has no entries.
func (p *GitHubPagesPublisher) ManifestTitles() ([]string, error) {
	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return nil, fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	if _, err := p.ensureRepo(auth); err != nil {
		return nil, fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	manifestAbsPath := filepath.Join(p.cfg.CloneDir, outputDir, ManifestFilename)
	manifest, err := LoadManifest(manifestAbsPath)
	if err != nil {
		return nil, fmt.Errorf("github pages: load manifest: %w", err)
	}

	titles := make([]string, 0, len(manifest.Digests))
	for _, e := range manifest.Digests {
		titles = append(titles, e.Title)
	}
	return titles, nil
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

// ensureIndex writes the digest index under outputDir and a root index.html
// that renders the same archive while loading manifest and digest files from
// outputDir.
func (p *GitHubPagesPublisher) ensureIndex(wt *gogit.Worktree, outputDir string) error {
	indexRelPath := filepath.Join(outputDir, "index.html")
	indexAbsPath := filepath.Join(p.cfg.CloneDir, indexRelPath)

	indexBytes, err := RenderDigestIndex()
	if err != nil {
		return fmt.Errorf("github pages: failed to build index: %w", err)
	}

	existing, readErr := os.ReadFile(indexAbsPath)
	if readErr != nil || !bytes.Equal(existing, indexBytes) {
		if err := os.MkdirAll(filepath.Dir(indexAbsPath), 0755); err != nil {
			return fmt.Errorf("github pages: failed to create index dir: %w", err)
		}
		if err := os.WriteFile(indexAbsPath, indexBytes, 0644); err != nil {
			return fmt.Errorf("github pages: failed to write index HTML: %w", err)
		}
		if _, err := wt.Add(indexRelPath); err != nil {
			return fmt.Errorf("github pages: failed to stage index file: %w", err)
		}
	}

	rootIndexRelPath := "index.html"
	rootIndexAbsPath := filepath.Join(p.cfg.CloneDir, rootIndexRelPath)
	rootIndexBytes, err := renderDigestIndexWithPaths(
		filepath.ToSlash(filepath.Join(outputDir, ManifestFilename)),
		filepath.ToSlash(outputDir),
	)
	if err != nil {
		return fmt.Errorf("github pages: failed to build root index: %w", err)
	}
	existingRoot, readRootErr := os.ReadFile(rootIndexAbsPath)
	if readRootErr == nil && bytes.Equal(existingRoot, rootIndexBytes) {
		return nil
	}
	if err := os.WriteFile(rootIndexAbsPath, rootIndexBytes, 0644); err != nil {
		return fmt.Errorf("github pages: failed to write root index HTML: %w", err)
	}
	if _, err := wt.Add(rootIndexRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage root index file: %w", err)
	}
	return nil
}

func isNonFastForward(err error) bool {
	return err != nil && strings.Contains(err.Error(), "non-fast-forward")
}

func (p *GitHubPagesPublisher) writeAndStageManifest(wt *gogit.Worktree, digest models.Digest, outputDir string) error {
	manifestRelPath := filepath.Join(outputDir, ManifestFilename)
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
	return nil
}

// InitPages sets up (or re-initialises) the GitHub Pages repository.
//
// It creates the remote branch as a clean orphan if it is absent, optionally
// configures the GitHub Pages source via the API (when configure_pages is
// true), clones the branch locally, and seeds the initial static files
// (manifest.json and index pages). Files that already exist are not
// overwritten; the operation is idempotent.
//
// When reinit is true the remote branch and the local clone directory are
// deleted before re-creating them from scratch.
func (p *GitHubPagesPublisher) InitPages(reinit bool) error {
	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	owner, repoName, err := parseGitHubRepoURL(p.cfg.RepoURL)
	if err != nil {
		return err
	}

	if reinit {
		log.WithFields(log.Fields{"owner": owner, "repo": repoName, "branch": p.cfg.Branch}).
			Info("Deleting remote branch for reinitialisation")
		if err := p.deleteRemoteBranchIfExists(owner, repoName); err != nil {
			return fmt.Errorf("github pages: delete branch: %w", err)
		}
		if err := os.RemoveAll(p.cfg.CloneDir); err != nil {
			return fmt.Errorf("github pages: remove clone dir: %w", err)
		}
	}

	if err := p.ensureRemoteBranchExists(owner, repoName); err != nil {
		return fmt.Errorf("github pages: ensure branch %q exists: %w", p.cfg.Branch, err)
	}

	gitRepo, err := p.ensureRepo(auth)
	if err != nil {
		return fmt.Errorf("github pages: prepare local repo: %w", err)
	}

	wt, err := gitRepo.Worktree()
	if err != nil {
		return fmt.Errorf("github pages: get worktree: %w", err)
	}

	manifestRelPath := filepath.Join(outputDir, ManifestFilename)
	manifestAbsPath := filepath.Join(p.cfg.CloneDir, manifestRelPath)
	if reinit || !fileExists(manifestAbsPath) {
		m, err := LoadManifest(manifestAbsPath)
		if err != nil {
			return fmt.Errorf("github pages: init manifest: %w", err)
		}
		if err := m.Write(manifestAbsPath); err != nil {
			return fmt.Errorf("github pages: write manifest: %w", err)
		}
		if _, err := wt.Add(manifestRelPath); err != nil {
			return fmt.Errorf("github pages: stage manifest: %w", err)
		}
	}

	if err := p.ensureIndex(wt, outputDir); err != nil {
		return err
	}

	status, err := wt.Status()
	if err != nil {
		return fmt.Errorf("github pages: get status: %w", err)
	}
	hasStaged := false
	for _, s := range status {
		if s.Staging != gogit.Unmodified {
			hasStaged = true
			break
		}
	}
	if !hasStaged {
		log.Info("GitHub Pages: nothing to commit — repository already initialised")
		if p.cfg.ConfigurePages {
			if err := p.configureGitHubPagesSource(); err != nil {
				return fmt.Errorf("github pages: configure source: %w", err)
			}
		}
		return nil
	}

	commitMsg := "Initialize GitHub Pages structure"
	if reinit {
		commitMsg = "Reinitialize GitHub Pages structure"
	}
	if _, err := wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	}); err != nil {
		return fmt.Errorf("github pages: commit: %w", err)
	}

	if err := gitRepo.Push(&gogit.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	}); err != nil {
		return fmt.Errorf("github pages: push: %w", err)
	}

	if p.cfg.ConfigurePages {
		if err := p.configureGitHubPagesSource(); err != nil {
			return fmt.Errorf("github pages: configure source: %w", err)
		}
	}

	log.WithField("branch", p.cfg.Branch).Info("GitHub Pages initialised successfully")
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func resolveGitHubPagesOutputDir(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "digests", nil
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("must be a relative path")
	}
	for _, part := range strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == ".." {
			return "", fmt.Errorf("must not contain parent traversal")
		}
	}
	cleaned := filepath.Clean(trimmed)
	if cleaned == "." || cleaned == ".." || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("must be a safe relative subdirectory")
	}
	return cleaned, nil
}

// RepublishAll re-renders every digest with the current templates and pushes
// the result as a single commit. The manifest is rebuilt from scratch so stale
// entries are removed. Pass dryRun=true to render and stage locally without
// committing or pushing. When wait is true (and not a dry run) it blocks until
// the resulting GitHub Pages build deploys.
func (p *GitHubPagesPublisher) RepublishAll(digests []models.Digest, theme string, dryRun, wait bool) error {
	if len(digests) == 0 {
		log.Info("RepublishAll: no digests to republish")
		return nil
	}

	log.WithField("count", len(digests)).Info("Republishing all digests to GitHub Pages")

	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	p.pStart("prepare", "Preparing Pages repo")
	repo, err := p.ensureRepo(auth)
	if err != nil {
		p.pComplete("prepare", false, "clone/pull failed")
		return fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		p.pComplete("prepare", false, "worktree failed")
		return fmt.Errorf("github pages: failed to get worktree: %w", err)
	}
	p.pComplete("prepare", true, "repo ready")

	// Load existing manifest to get the set of already-published filenames,
	// then rebuild Digests cleanly from only those digests.
	manifestRelPath := filepath.Join(outputDir, ManifestFilename)
	manifestAbsPath := filepath.Join(p.cfg.CloneDir, manifestRelPath)
	manifest, err := LoadManifest(manifestAbsPath)
	if err != nil {
		return fmt.Errorf("github pages: load manifest: %w", err)
	}
	published := make(map[string]bool, len(manifest.Digests))
	for _, e := range manifest.Digests {
		published[e.Filename] = true
	}
	var toRender []models.Digest
	for _, d := range digests {
		if published[DigestHTMLFilename(d)] {
			toRender = append(toRender, d)
		}
	}
	if len(toRender) == 0 {
		log.Info("RepublishAll: no server digests match the published manifest — nothing to do")
		return nil
	}
	log.WithFields(log.Fields{"published": len(published), "matched": len(toRender)}).
		Info("Filtered to published digests only")

	manifest.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	manifest.Digests = nil

	// Phase 1: render and write HTML files in parallel (template execution is CPU-bound).
	// wt.Add (git index) is not goroutine-safe, so staging happens after this phase.
	type renderedPaths struct {
		digestRelPath string
		swipeRelPath  string
	}
	paths := make([]renderedPaths, len(toRender))

	workers := max(runtime.NumCPU()-1, 1)
	log.WithFields(log.Fields{"count": len(toRender), "workers": workers}).Info("Rendering digests in parallel")
	p.pStart("render", fmt.Sprintf("Rendering %d pages", len(toRender)))

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(workers)

	for i, digest := range toRender {
		g.Go(func() error {
			log.WithFields(log.Fields{"digestId": digest.Id, "index": i + 1, "total": len(toRender)}).
				Info("Rendering digest")

			digestFilename := DigestHTMLFilename(digest)
			digestRelPath := filepath.Join(outputDir, digestFilename)
			digestAbsPath := filepath.Join(p.cfg.CloneDir, digestRelPath)

			htmlBytes, err := RenderDigestHTML(digest, theme)
			if err != nil {
				return fmt.Errorf("github pages: render digest %s: %w", digest.Id, err)
			}
			if err := os.MkdirAll(filepath.Dir(digestAbsPath), 0755); err != nil {
				return fmt.Errorf("github pages: create output dir: %w", err)
			}
			if err := os.WriteFile(digestAbsPath, htmlBytes, 0644); err != nil {
				return fmt.Errorf("github pages: write digest HTML %s: %w", digest.Id, err)
			}

			swipeBytes, err := RenderSwipeHTML(digest, digestFilename)
			if err != nil {
				return fmt.Errorf("github pages: render swipe %s: %w", digest.Id, err)
			}
			swipeRelPath := filepath.Join(outputDir, SwipeHTMLFilename(digest))
			swipeAbsPath := filepath.Join(p.cfg.CloneDir, swipeRelPath)
			if err := os.WriteFile(swipeAbsPath, swipeBytes, 0644); err != nil {
				return fmt.Errorf("github pages: write swipe HTML %s: %w", digest.Id, err)
			}

			paths[i] = renderedPaths{digestRelPath: digestRelPath, swipeRelPath: swipeRelPath}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		p.pComplete("render", false, "render failed")
		return err
	}

	// Phase 2: stage all rendered files and update the manifest sequentially.
	for i, rp := range paths {
		if _, err := wt.Add(rp.digestRelPath); err != nil {
			return fmt.Errorf("github pages: stage digest file: %w", err)
		}
		if _, err := wt.Add(rp.swipeRelPath); err != nil {
			return fmt.Errorf("github pages: stage swipe file: %w", err)
		}
		manifest.Upsert(ManifestEntryFromDigest(toRender[i]))
	}

	if err := manifest.Write(manifestAbsPath); err != nil {
		return fmt.Errorf("github pages: write manifest: %w", err)
	}
	if _, err := wt.Add(manifestRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage manifest: %w", err)
	}

	if err := p.writeAndStageFeeds(wt, outputDir, mergeDigestsNewestFirst(toRender, FeedDigestLimit)); err != nil {
		p.pComplete("render", false, "feed render failed")
		return err
	}

	if err := p.ensureIndex(wt, outputDir); err != nil {
		p.pComplete("render", false, "index render failed")
		return err
	}
	p.pComplete("render", true, fmt.Sprintf("rendered %d pages", len(toRender)))

	if dryRun {
		log.WithField("count", len(toRender)).Info("Dry run complete — skipping commit and push")
		p.pStart("commit", "Committing & pushing")
		p.pComplete("commit", true, "dry run — not pushed")
		return nil
	}

	p.pStart("commit", "Committing & pushing")
	commitMsg := fmt.Sprintf("Republish %d digests (template migration)", len(toRender))
	commitHash, err := wt.Commit(commitMsg, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		p.pComplete("commit", false, "commit failed")
		return fmt.Errorf("github pages: failed to commit: %w", err)
	}

	if err := p.pushWithRetry(repo, wt, auth); err != nil {
		p.pComplete("commit", false, "push failed")
		return err
	}
	p.pComplete("commit", true, "pushed "+shortSHA(commitHash.String()))

	log.WithField("count", len(toRender)).Info("Published digests republished to GitHub Pages")
	return p.waitForDeploy(commitHash.String(), "deploy", "Waiting for GitHub Pages deploy", wait)
}

// RepublishIndex re-renders the archive index pages with the current templates
// and pushes the result as a single commit. The manifest and digest HTML files
// are not touched. Pass dryRun=true to write locally without committing. When
// wait is true (and not a dry run) it blocks until the resulting GitHub Pages
// build deploys.
func (p *GitHubPagesPublisher) RepublishIndex(dryRun, wait bool) error {
	outputDir, err := resolveGitHubPagesOutputDir(p.cfg.OutputDir)
	if err != nil {
		return fmt.Errorf("github pages: invalid output dir: %w", err)
	}

	auth := &githttp.BasicAuth{
		Username: "x-access-token",
		Password: p.cfg.Token,
	}

	p.pStart("prepare", "Preparing Pages repo")
	repo, err := p.ensureRepo(auth)
	if err != nil {
		p.pComplete("prepare", false, "clone/pull failed")
		return fmt.Errorf("github pages: failed to prepare local repo: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		p.pComplete("prepare", false, "worktree failed")
		return fmt.Errorf("github pages: failed to get worktree: %w", err)
	}
	p.pComplete("prepare", true, "repo ready")

	p.pStart("render", "Rendering index pages")
	if err := p.ensureIndex(wt, outputDir); err != nil {
		p.pComplete("render", false, "render failed")
		return err
	}

	status, err := wt.Status()
	if err != nil {
		p.pComplete("render", false, "status failed")
		return fmt.Errorf("github pages: failed to get worktree status: %w", err)
	}
	hasStaged := false
	for _, s := range status {
		if s.Staging != gogit.Unmodified {
			hasStaged = true
			break
		}
	}
	if !hasStaged {
		log.Info("RepublishIndex: index pages already up to date — nothing to commit")
		p.pComplete("render", true, "already up to date")
		return nil
	}
	p.pComplete("render", true, "rendered index pages")

	if dryRun {
		log.Info("Dry run complete — skipping commit and push")
		p.pStart("commit", "Committing & pushing")
		p.pComplete("commit", true, "dry run — not pushed")
		return nil
	}

	p.pStart("commit", "Committing & pushing")
	commitHash, err := wt.Commit("Republish index pages (template migration)", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  p.cfg.CommitAuthor,
			Email: p.cfg.CommitEmail,
			When:  time.Now(),
		},
	})
	if err != nil {
		p.pComplete("commit", false, "commit failed")
		return fmt.Errorf("github pages: failed to commit: %w", err)
	}

	if err := p.pushWithRetry(repo, wt, auth); err != nil {
		p.pComplete("commit", false, "push failed")
		return err
	}
	p.pComplete("commit", true, "pushed "+shortSHA(commitHash.String()))

	log.Info("Index pages republished to GitHub Pages")
	return p.waitForDeploy(commitHash.String(), "deploy", "Waiting for GitHub Pages deploy", wait)
}

// pushWithRetry pushes and retries once after pulling on non-fast-forward rejection.
func (p *GitHubPagesPublisher) pushWithRetry(repo *gogit.Repository, wt *gogit.Worktree, auth *githttp.BasicAuth) error {
	pushOpts := &gogit.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
	}
	if err := repo.Push(pushOpts); err != nil {
		if !isNonFastForward(err) {
			return fmt.Errorf("github pages: push failed: %w", err)
		}
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
	}
	return nil
}

// renderAndStageSwipe renders the swipe triage view, writes it alongside the
// digest HTML, and stages it in the worktree.
func (p *GitHubPagesPublisher) renderAndStageSwipe(wt *gogit.Worktree, digest models.Digest, outputDir string) error {
	digestFilename := DigestHTMLFilename(digest)
	swipeBytes, err := RenderSwipeHTML(digest, digestFilename)
	if err != nil {
		return fmt.Errorf("github pages: failed to render swipe HTML: %w", err)
	}

	swipeFilename := SwipeHTMLFilename(digest)
	swipeRelPath := filepath.Join(outputDir, swipeFilename)
	swipeAbsPath := filepath.Join(p.cfg.CloneDir, swipeRelPath)

	if err := os.WriteFile(swipeAbsPath, swipeBytes, 0644); err != nil {
		return fmt.Errorf("github pages: failed to write swipe HTML: %w", err)
	}

	if _, err := wt.Add(swipeRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage swipe file: %w", err)
	}
	return nil
}

// renderAndStage renders a digest, writes it to the publisher's output dir,
// and stages it in the worktree. It returns the staged file's repo-relative
// path.
func (p *GitHubPagesPublisher) renderAndStage(wt *gogit.Worktree, digest models.Digest, outputDir string, theme string) (string, error) {
	htmlBytes, err := RenderDigestHTML(digest, theme)
	if err != nil {
		return "", fmt.Errorf("github pages: failed to render digest HTML: %w", err)
	}

	digestFilename := DigestHTMLFilename(digest)
	digestRelPath := filepath.Join(outputDir, digestFilename)
	digestAbsPath := filepath.Join(p.cfg.CloneDir, digestRelPath)

	if err := os.MkdirAll(filepath.Dir(digestAbsPath), 0755); err != nil {
		return "", fmt.Errorf("github pages: failed to create output dir: %w", err)
	}

	if err := os.WriteFile(digestAbsPath, htmlBytes, 0644); err != nil {
		return "", fmt.Errorf("github pages: failed to write digest HTML: %w", err)
	}

	if _, err := wt.Add(digestRelPath); err != nil {
		return "", fmt.Errorf("github pages: failed to stage digest file: %w", err)
	}
	return digestRelPath, nil
}
