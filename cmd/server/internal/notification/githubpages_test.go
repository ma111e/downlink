package notification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"downlink/pkg/models"

	gogit "gopkg.in/src-d/go-git.v4"
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
	digest := sampleDigest("digest-one", time.Date(2026, 4, 24, 12, 0, 0, 0, time.UTC))

	relPath, err := publisher.renderAndStage(wt, digest, outputDir)
	if err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if relPath != filepath.Join("digests", "downlink-digest-2026-04-24_1200.html") {
		t.Fatalf("relPath = %q", relPath)
	}
	if err := publisher.writeAndStageManifest(wt, digest, outputDir); err != nil {
		t.Fatalf("writeAndStageManifest() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	assertFileExists(t, cloneDir, "digests", "downlink-digest-2026-04-24_1200.html")
	assertManifestContains(t, cloneDir, "digests", "manifest.json", "downlink-digest-2026-04-24_1200.html")
	assertFileExists(t, cloneDir, "digests", "index.html")
	rootIndex := assertFileExists(t, cloneDir, "index.html")
	if !strings.Contains(string(rootIndex), "digests/index.html") {
		t.Fatalf("root index does not point at digests/index.html:\n%s", string(rootIndex))
	}
	assertStaged(t, wt,
		filepath.Join("digests", "downlink-digest-2026-04-24_1200.html"),
		filepath.Join("digests", "manifest.json"),
		filepath.Join("digests", "index.html"),
		"index.html",
	)
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
	digest := sampleDigest("digest-two", time.Date(2026, 4, 25, 9, 30, 0, 0, time.UTC))

	relPath, err := publisher.renderAndStage(wt, digest, outputDir)
	if err != nil {
		t.Fatalf("renderAndStage() error = %v", err)
	}
	if relPath != filepath.Join("archive", "digests", "downlink-digest-2026-04-25_0930.html") {
		t.Fatalf("relPath = %q", relPath)
	}
	if err := publisher.writeAndStageManifest(wt, digest, outputDir); err != nil {
		t.Fatalf("writeAndStageManifest() error = %v", err)
	}
	if err := publisher.ensureIndex(wt, outputDir); err != nil {
		t.Fatalf("ensureIndex() error = %v", err)
	}

	assertFileExists(t, cloneDir, "archive", "digests", "downlink-digest-2026-04-25_0930.html")
	assertManifestContains(t, cloneDir, "archive", "digests", "manifest.json", "downlink-digest-2026-04-25_0930.html")
	assertFileExists(t, cloneDir, "archive", "digests", "index.html")
	rootIndex := assertFileExists(t, cloneDir, "index.html")
	if !strings.Contains(string(rootIndex), "archive/digests/index.html") {
		t.Fatalf("root index does not point at archive/digests/index.html:\n%s", string(rootIndex))
	}
	assertStaged(t, wt,
		filepath.Join("archive", "digests", "downlink-digest-2026-04-25_0930.html"),
		filepath.Join("archive", "digests", "manifest.json"),
		filepath.Join("archive", "digests", "index.html"),
		"index.html",
	)
}

func sampleDigest(id string, createdAt time.Time) models.Digest {
	count := 2
	return models.Digest{
		Id:            id,
		CreatedAt:     createdAt,
		ArticleCount:  &count,
		TimeWindow:    24 * time.Hour,
		DigestSummary: "## Summary\n\nA short digest.",
		ProviderResults: []models.DigestProviderResult{
			{ProviderType: "openai", ModelName: "gpt-test"},
		},
		Articles: []models.Article{
			{Id: "article-b", Title: "Article B", Link: "https://example.com/b", PublishedAt: createdAt},
			{Id: "article-a", Title: "Article A", Link: "https://example.com/a", PublishedAt: createdAt},
		},
		DigestAnalyses: []models.DigestAnalysis{
			{
				ArticleId: "article-b",
				Analysis: &models.ArticleAnalysis{
					ArticleId:       "article-b",
					ProviderType:    "openai",
					ModelName:       "gpt-test",
					ImportanceScore: 95,
				},
			},
			{
				ArticleId: "article-a",
				Analysis: &models.ArticleAnalysis{
					ArticleId:       "article-a",
					ProviderType:    "openai",
					ModelName:       "gpt-test",
					ImportanceScore: 80,
				},
			},
		},
	}
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
