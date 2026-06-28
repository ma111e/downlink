package notification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ma111e/downlink/pkg/models"

	gogit "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

func TestResolveGitHubPagesOutputDir(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "digests"},
		{name: "trims default", input: "  ", want: "digests"},
		{name: "custom", input: "archive/digests", want: filepath.Join("archive", "digests")},
		{name: "cleans custom", input: "archive/./digests", want: filepath.Join("archive", "digests")},
		{name: "absolute", input: "/tmp/digests", wantErr: true},
		{name: "dot", input: ".", wantErr: true},
		{name: "parent", input: "..", wantErr: true},
		{name: "parent traversal", input: "../digests", wantErr: true},
		{name: "middle parent traversal", input: "archive/../digests", wantErr: true},
		{name: "nested parent traversal", input: "archive/../../digests", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveGitHubPagesOutputDir(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveGitHubPagesOutputDir() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveGitHubPagesOutputDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGitHubPagesPublisherWritesDefaultDigestFolderLayout(t *testing.T) {
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{CloneDir: cloneDir})
	outputDir, err := resolveGitHubPagesOutputDir(publisher.cfg.OutputDir)
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	digest := sampleDigest("digest-one", time.Now().UTC().Truncate(time.Minute))
	digestFilename := DigestHTMLFilename(digest)

	relPath, err := publisher.renderAndStage(wt, digest, outputDir, "default")
	if err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if relPath != filepath.Join("digests", digestFilename) {
		t.Fatalf("relPath = %q", relPath)
	}
	if err := publisher.writeAndStageManifest(wt, digest, outputDir); err != nil {
		t.Fatalf("writeAndStageManifest() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir, "default"); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	assertFileExists(t, cloneDir, "digests", digestFilename)
	assertManifestContains(t, cloneDir, "digests", "manifest.json", digestFilename)
	assertFileExists(t, cloneDir, "digests", "index.html")
	rootIndex := assertFileExists(t, cloneDir, "index.html")
	if !strings.Contains(string(rootIndex), `data-manifest-url="digests/manifest.json"`) ||
		!strings.Contains(string(rootIndex), `data-digest-base-url="digests"`) ||
		!strings.Contains(string(rootIndex), "DOWNLINK") {
		t.Fatalf("root index is not the archive shell for digests:\n%s", string(rootIndex))
	}
	assertStaged(t, wt,
		filepath.Join("digests", digestFilename),
		filepath.Join("digests", "manifest.json"),
		filepath.Join("digests", "index.html"),
		"index.html",
	)
}

func TestGitHubPagesPublisherWritesExternalStylesheets(t *testing.T) {
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}

	// Default config => external CSS.
	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{CloneDir: cloneDir})
	outputDir, err := resolveGitHubPagesOutputDir(publisher.cfg.OutputDir)
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	digest := sampleDigest("digest-ext", time.Now().UTC().Truncate(time.Minute))
	digestFilename := DigestHTMLFilename(digest)

	if _, err := publisher.renderAndStage(wt, digest, outputDir, "default"); err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir, "default"); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	// The CSS files land at both the repo root and the digest folder, and are staged.
	for _, name := range stylesheetAssets {
		assertFileExists(t, cloneDir, name)
		assertFileExists(t, cloneDir, "digests", name)
		assertStaged(t, wt, name, filepath.Join("digests", name))
	}
	// The digest page links the external sheet rather than inlining the rules.
	page := assertFileExists(t, cloneDir, "digests", digestFilename)
	if !strings.Contains(string(page), `<link rel="stylesheet" href="./digest.css">`) {
		t.Fatalf("external digest page should link ./digest.css")
	}
	if strings.Contains(string(page), ".toc-title") {
		t.Fatalf("external digest page should not inline the stylesheet rules")
	}
}

func TestGitHubPagesPublisherSelfContainedInlinesCSS(t *testing.T) {
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{CloneDir: cloneDir, SelfContained: true})
	outputDir, err := resolveGitHubPagesOutputDir(publisher.cfg.OutputDir)
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	digest := sampleDigest("digest-sc", time.Now().UTC().Truncate(time.Minute))
	digestFilename := DigestHTMLFilename(digest)

	if _, err := publisher.renderAndStage(wt, digest, outputDir, "default"); err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir, "default"); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	// No external CSS files are written.
	for _, name := range stylesheetAssets {
		if _, err := os.Stat(filepath.Join(cloneDir, name)); !os.IsNotExist(err) {
			t.Fatalf("self-contained mode should not write %s (err=%v)", name, err)
		}
		if _, err := os.Stat(filepath.Join(cloneDir, "digests", name)); !os.IsNotExist(err) {
			t.Fatalf("self-contained mode should not write digests/%s (err=%v)", name, err)
		}
	}
	// The digest page inlines its CSS and carries no external link.
	page := assertFileExists(t, cloneDir, "digests", digestFilename)
	if strings.Contains(string(page), `href="./digest.css"`) {
		t.Fatalf("self-contained digest page should not link ./digest.css")
	}
	if !strings.Contains(string(page), ".toc-title") {
		t.Fatalf("self-contained digest page should inline the stylesheet rules")
	}
}

func TestGitHubPagesPublisherWritesCustomDigestFolderLayout(t *testing.T) {
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		CloneDir:  cloneDir,
		OutputDir: "archive/digests",
	})
	outputDir, err := resolveGitHubPagesOutputDir(publisher.cfg.OutputDir)
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}
	digest := sampleDigest("digest-two", time.Now().UTC().Truncate(time.Minute))
	digestFilename := DigestHTMLFilename(digest)

	relPath, err := publisher.renderAndStage(wt, digest, outputDir, "default")
	if err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if relPath != filepath.Join("archive", "digests", digestFilename) {
		t.Fatalf("relPath = %q", relPath)
	}
	if err := publisher.writeAndStageManifest(wt, digest, outputDir); err != nil {
		t.Fatalf("writeAndStageManifest() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir, "default"); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	assertFileExists(t, cloneDir, "archive", "digests", digestFilename)
	assertManifestContains(t, cloneDir, "archive", "digests", "manifest.json", digestFilename)
	assertFileExists(t, cloneDir, "archive", "digests", "index.html")
	rootIndex := assertFileExists(t, cloneDir, "index.html")
	if !strings.Contains(string(rootIndex), `data-manifest-url="archive/digests/manifest.json"`) ||
		!strings.Contains(string(rootIndex), `data-digest-base-url="archive/digests"`) ||
		!strings.Contains(string(rootIndex), "DOWNLINK") {
		t.Fatalf("root index is not the archive shell for archive/digests:\n%s", string(rootIndex))
	}
	assertStaged(t, wt,
		filepath.Join("archive", "digests", digestFilename),
		filepath.Join("archive", "digests", "manifest.json"),
		filepath.Join("archive", "digests", "index.html"),
		"index.html",
	)
}

// sampleDigest is a test-local alias for the exported SampleDigest fixture
// (see sample.go), which is shared with the digest dev server.
func TestPruneAgedDigestFiles(t *testing.T) {
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit() error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree() error = %v", err)
	}

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{CloneDir: cloneDir})
	outputDir, err := resolveGitHubPagesOutputDir(publisher.cfg.OutputDir)
	if err != nil {
		t.Fatalf("resolve output dir: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Minute)
	cutoff := now.AddDate(0, 0, -30)
	aged := sampleDigest("aged", now.AddDate(0, 0, -40))
	fresh := sampleDigest("fresh", now.AddDate(0, 0, -10))

	// Seed digest + swipe files for both digests and stage them.
	var agedFiles, freshFiles []string
	for _, f := range []struct {
		d     models.Digest
		files *[]string
	}{{aged, &agedFiles}, {fresh, &freshFiles}} {
		for _, name := range []string{DigestHTMLFilename(f.d), SwipeHTMLFilename(f.d)} {
			rel := filepath.Join(outputDir, name)
			if err := os.MkdirAll(filepath.Join(cloneDir, outputDir), 0755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(cloneDir, rel), []byte("<html></html>"), 0644); err != nil {
				t.Fatalf("write %s: %v", rel, err)
			}
			if _, err := wt.Add(rel); err != nil {
				t.Fatalf("stage %s: %v", rel, err)
			}
			*f.files = append(*f.files, rel)
		}
	}

	if err := publisher.pruneAgedDigestFiles(wt, outputDir, cutoff); err != nil {
		t.Fatalf("pruneAgedDigestFiles() error = %v", err)
	}

	for _, rel := range agedFiles {
		if _, err := os.Stat(filepath.Join(cloneDir, rel)); !os.IsNotExist(err) {
			t.Fatalf("aged file %s should have been removed, stat err = %v", rel, err)
		}
	}
	for _, rel := range freshFiles {
		if _, err := os.Stat(filepath.Join(cloneDir, rel)); err != nil {
			t.Fatalf("in-window file %s should remain: %v", rel, err)
		}
	}
}

func TestCommitOrphanAndForcePush(t *testing.T) {
	const branch = "main"

	// Bare remote.
	bareDir := t.TempDir()
	if _, err := gogit.PlainInit(bareDir, true); err != nil {
		t.Fatalf("PlainInit(bare) error = %v", err)
	}

	// Seeded clone with a two-commit history on the target branch, pushed to the
	// bare remote so there is real prior history to overwrite.
	cloneDir := t.TempDir()
	repo, err := gogit.PlainInit(cloneDir, false)
	if err != nil {
		t.Fatalf("PlainInit(clone) error = %v", err)
	}
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))); err != nil {
		t.Fatalf("SetReference error = %v", err)
	}
	if _, err := repo.CreateRemote(&config.RemoteConfig{Name: "origin", URLs: []string{bareDir}}); err != nil {
		t.Fatalf("CreateRemote error = %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree error = %v", err)
	}
	sig := &object.Signature{Name: "seed", Email: "seed@example.com", When: time.Now()}
	for i, content := range []string{"first", "second"} {
		if err := os.WriteFile(filepath.Join(cloneDir, "index.html"), []byte(content), 0644); err != nil {
			t.Fatalf("write index.html: %v", err)
		}
		if _, err := wt.Add("index.html"); err != nil {
			t.Fatalf("stage index.html: %v", err)
		}
		if _, err := wt.Commit(fmt.Sprintf("seed commit %d", i), &gogit.CommitOptions{Author: sig}); err != nil {
			t.Fatalf("seed commit: %v", err)
		}
	}
	pushSpec := config.RefSpec(fmt.Sprintf("+refs/heads/%s:refs/heads/%s", branch, branch))
	if err := repo.Push(&gogit.PushOptions{RemoteName: "origin", RefSpecs: []config.RefSpec{pushSpec}}); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	// The rendered tree the orphan commit should capture from disk.
	if err := os.WriteFile(filepath.Join(cloneDir, "index.html"), []byte("rebuilt"), 0644); err != nil {
		t.Fatalf("write rebuilt index.html: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cloneDir, "manifest.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}

	publisher := NewGitHubPagesPublisher(models.GitHubPagesNotificationConfig{
		RepoURL:      bareDir,
		CloneDir:     cloneDir,
		Branch:       branch,
		CommitAuthor: "downlink-bot",
		CommitEmail:  "bot@example.com",
	})

	assertOrphanTip := func(wantContent string) {
		t.Helper()
		hash, err := publisher.commitOrphanAndForcePush(nil, "rebuild")
		if err != nil {
			t.Fatalf("commitOrphanAndForcePush() error = %v", err)
		}
		remote, err := gogit.PlainOpen(bareDir)
		if err != nil {
			t.Fatalf("open bare: %v", err)
		}
		ref, err := remote.Reference(plumbing.NewBranchReferenceName(branch), true)
		if err != nil {
			t.Fatalf("resolve remote branch: %v", err)
		}
		if ref.Hash() != hash {
			t.Fatalf("remote tip = %s, want pushed hash %s", ref.Hash(), hash)
		}
		commit, err := remote.CommitObject(ref.Hash())
		if err != nil {
			t.Fatalf("commit object: %v", err)
		}
		if commit.NumParents() != 0 {
			t.Fatalf("orphan commit has %d parents, want 0", commit.NumParents())
		}
		for _, name := range []string{"index.html", "manifest.json"} {
			if _, err := commit.File(name); err != nil {
				t.Fatalf("expected %s in orphan commit: %v", name, err)
			}
		}
		f, err := commit.File("index.html")
		if err != nil {
			t.Fatalf("index.html: %v", err)
		}
		got, err := f.Contents()
		if err != nil {
			t.Fatalf("contents: %v", err)
		}
		if got != wantContent {
			t.Fatalf("index.html content = %q, want %q", got, wantContent)
		}
	}

	// First rebuild overwrites the two-commit history with a single orphan commit.
	assertOrphanTip("rebuilt")

	// A second rebuild force-pushes again over the prior orphan tip without error.
	if err := os.WriteFile(filepath.Join(cloneDir, "index.html"), []byte("rebuilt-again"), 0644); err != nil {
		t.Fatalf("rewrite index.html: %v", err)
	}
	assertOrphanTip("rebuilt-again")
}

func sampleDigest(id string, createdAt time.Time) models.Digest {
	return SampleDigest(id, createdAt)
}

func assertFileExists(t *testing.T, parts ...string) []byte {
	t.Helper()
	path := filepath.Join(parts...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
	return data
}

func assertManifestContains(t *testing.T, parts ...string) {
	t.Helper()
	if len(parts) < 2 {
		t.Fatalf("assertManifestContains requires path parts plus filename")
	}
	filename := parts[len(parts)-1]
	pathParts := parts[:len(parts)-1]
	data := assertFileExists(t, pathParts...)
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("manifest json: %v", err)
	}
	for _, entry := range manifest.Digests {
		if entry.Filename == filename && entry.Provider == "openai" && entry.Model == "gpt-test" {
			return
		}
	}
	t.Fatalf("manifest does not contain filename=%q with new schema fields: %+v", filename, manifest.Digests)
}

func assertStaged(t *testing.T, wt *gogit.Worktree, paths ...string) {
	t.Helper()
	status, err := wt.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	for _, path := range paths {
		slashPath := filepath.ToSlash(path)
		fileStatus, ok := status[slashPath]
		if !ok {
			t.Fatalf("%s not present in git status:\n%s", slashPath, status.String())
		}
		if fileStatus.Staging != gogit.Added {
			t.Fatalf("%s staging status = %q, want %q; full status:\n%s", slashPath, fileStatus.Staging, gogit.Added, status.String())
		}
	}
}
