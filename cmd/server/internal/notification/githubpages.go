package notification

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"downlink/pkg/models"

	log "github.com/sirupsen/logrus"
	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	githttp "gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

// DigestSiblingStore is the minimal store surface the publisher needs to find
// and fully load sibling digests when rebuilding switcher links.
type DigestSiblingStore interface {
	GetDigest(id string) (models.Digest, error)
	FindDigestsWithSameArticleSet(digestId string) ([]models.Digest, error)
}

// GitHubPagesPublisher publishes digest HTML files to a GitHub Pages repository.
type GitHubPagesPublisher struct {
	cfg   models.GitHubPagesNotificationConfig
	store DigestSiblingStore
}

// NewGitHubPagesPublisher creates a new GitHubPagesPublisher.
// store is used at publish time to discover sibling digests (same article set,
// different provider/model) so each affected page is re-rendered with an
// up-to-date switcher. It may be nil — in that case sibling rebuilds are
// skipped and pages are published without a switcher.
func NewGitHubPagesPublisher(cfg models.GitHubPagesNotificationConfig, store DigestSiblingStore) *GitHubPagesPublisher {
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
	return &GitHubPagesPublisher{cfg: cfg, store: store}
}

// SendDigest renders and publishes the digest HTML plus a regenerated index to the GitHub Pages repo.
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

	// Discover sibling digests (same exact article set) so the switcher is
	// rebuilt on every affected page in a single commit.
	var siblings []models.Digest
	if p.store != nil {
		var sErr error
		siblings, sErr = p.store.FindDigestsWithSameArticleSet(digest.Id)
		if sErr != nil {
			log.WithError(sErr).WithField("digestId", digest.Id).Warn("Failed to look up sibling digests; publishing without switcher")
			siblings = nil
		}
	}

	// Build the switcher entry list, ordered newest-first by created_at.
	// Always include the current digest, even when it isn't in the lookup
	// result yet (e.g. when the store is nil or returned an error).
	switcher := buildSwitcher(digest, siblings)

	// Render and stage the new digest with the updated switcher.
	digestRelPath, err := p.renderAndStage(wt, digest, switcher)
	if err != nil {
		return err
	}

	// Re-render every existing sibling so its switcher reflects the latest
	// membership. Skip the digest we just wrote — it's already up to date.
	siblingRebuilds := 0
	for _, s := range siblings {
		if s.Id == digest.Id {
			continue
		}
		full, loadErr := p.store.GetDigest(s.Id)
		if loadErr != nil {
			log.WithError(loadErr).WithField("siblingId", s.Id).Warn("Failed to reload sibling digest; skipping rebuild")
			continue
		}
		if _, err := p.renderAndStage(wt, full, switcher); err != nil {
			log.WithError(err).WithField("siblingId", s.Id).Warn("Failed to re-render sibling digest; skipping")
			continue
		}
		siblingRebuilds++
	}

	// Regenerate index.html.
	indexRelPath, indexBytes, err := p.buildIndex()
	if err != nil {
		return fmt.Errorf("github pages: failed to build index: %w", err)
	}
	indexAbsPath := filepath.Join(p.cfg.CloneDir, indexRelPath)
	if err := os.WriteFile(indexAbsPath, indexBytes, 0644); err != nil {
		return fmt.Errorf("github pages: failed to write index HTML: %w", err)
	}
	if _, err := wt.Add(indexRelPath); err != nil {
		return fmt.Errorf("github pages: failed to stage index file: %w", err)
	}

	commitMsg := fmt.Sprintf("Publish digest %s (%s)", digest.Id[:8], digest.CreatedAt.UTC().Format("2006-01-02"))
	if siblingRebuilds > 0 {
		commitMsg = fmt.Sprintf("%s [+%d sibling rebuild(s)]", commitMsg, siblingRebuilds)
	}
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

// buildIndex scans the output directory for digest HTML files, sorts them
// newest-first, and returns the relative index path + rendered index HTML bytes.
func (p *GitHubPagesPublisher) buildIndex() (string, []byte, error) {
	scanDir := p.cfg.CloneDir
	if p.cfg.OutputDir != "" {
		scanDir = filepath.Join(p.cfg.CloneDir, p.cfg.OutputDir)
	}

	entries, err := os.ReadDir(scanDir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read output dir: %w", err)
	}

	var indexEntries []DigestIndexEntry
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "index.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		indexEntries = append(indexEntries, DigestIndexEntry{
			Filename:    name,
			DisplayDate: filenameToDisplayDate(name),
		})
	}

	// Sort newest first (ISO timestamp prefix sorts lexicographically).
	sort.Slice(indexEntries, func(i, j int) bool {
		return indexEntries[i].Filename > indexEntries[j].Filename
	})

	indexBytes, err := RenderDigestIndex(indexEntries)
	if err != nil {
		return "", nil, err
	}

	indexRelPath := "index.html"
	if p.cfg.OutputDir != "" {
		indexRelPath = filepath.Join(p.cfg.OutputDir, "index.html")
	}
	return indexRelPath, indexBytes, nil
}

// filenameToDisplayDate extracts a human-readable date string from a digest filename.
// e.g. "downlink-digest-2026-04-24_1200.html" → "2026-04-24 12:00 UTC"
func filenameToDisplayDate(filename string) string {
	name := strings.TrimSuffix(filename, ".html")
	// Expected suffix after "downlink-digest-": YYYY-MM-DD_HHMM
	const prefix = "downlink-digest-"
	if !strings.HasPrefix(name, prefix) {
		return filename
	}
	datePart := strings.TrimPrefix(name, prefix) // "2026-04-24_1200"
	t, err := time.Parse("2006-01-02_1504", datePart)
	if err != nil {
		return filename
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func isNonFastForward(err error) bool {
	return err != nil && strings.Contains(err.Error(), "non-fast-forward")
}

// buildSwitcher constructs the DigestSibling list shown in the nav switcher,
// ordered newest-first by CreatedAt and marking which entry corresponds to
// the digest being rendered. The current digest is always present even if
// siblings doesn't contain it (e.g. store unavailable).
func buildSwitcher(current models.Digest, siblings []models.Digest) []DigestSibling {
	hasCurrent := false
	for _, s := range siblings {
		if s.Id == current.Id {
			hasCurrent = true
			break
		}
	}
	all := siblings
	if !hasCurrent {
		all = append([]models.Digest{current}, siblings...)
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})

	out := make([]DigestSibling, 0, len(all))
	for _, d := range all {
		providerType, modelName := digestSwitcherLabel(d)
		out = append(out, DigestSibling{
			Filename:     DigestHTMLFilename(d),
			ProviderType: providerType,
			ModelName:    modelName,
			DisplayDate:  d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
			IsCurrent:    d.Id == current.Id,
		})
	}
	return out
}

// digestSwitcherLabel picks a provider/model label for the switcher option.
// Falls back to placeholders when no provider results have been recorded yet.
func digestSwitcherLabel(d models.Digest) (providerType, modelName string) {
	for _, r := range d.ProviderResults {
		if r.ProviderType != "" {
			return r.ProviderType, r.ModelName
		}
	}
	return "unknown", "unknown"
}

// renderAndStage renders a digest with the given switcher list, writes it to
// the publisher's output dir, and stages it in the worktree. It returns the
// staged file's repo-relative path.
func (p *GitHubPagesPublisher) renderAndStage(wt *gogit.Worktree, digest models.Digest, switcher []DigestSibling) (string, error) {
	htmlBytes, err := RenderDigestHTMLWithSiblings(digest, "dark", switcher)
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
