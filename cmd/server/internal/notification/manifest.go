package notification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"downlink/pkg/models"
)

// ManifestVersion is the current schema version written to manifest.json.
const ManifestVersion = 1

// ManifestFilename is the basename of the manifest checked into the Pages branch.
const ManifestFilename = "manifest.json"

// ManifestEntry describes a single published digest in the manifest.
type ManifestEntry struct {
	Id             string `json:"id"`
	Filename       string `json:"filename"`
	DisplayDate    string `json:"displayDate"`
	ProviderType   string `json:"providerType"`
	ModelName      string `json:"modelName"`
	ArticleSetHash string `json:"articleSetHash"`
}

// Manifest is the JSON document checked into the Pages branch listing every
// published digest. The browser-side switcher and index both read it directly.
type Manifest struct {
	Version   int             `json:"version"`
	UpdatedAt time.Time       `json:"updatedAt"`
	Digests   []ManifestEntry `json:"digests"`
}

// LoadManifest reads the manifest at path. When the file is missing, it falls
// back to a directory backfill: scans the parent directory for existing digest
// HTML files and synthesizes minimal entries (no providerType/modelName/hash).
// This keeps existing repos browseable on the first publish after the upgrade.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var m Manifest
		if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
			return Manifest{}, fmt.Errorf("parse manifest: %w", jsonErr)
		}
		if m.Version == 0 {
			m.Version = ManifestVersion
		}
		return m, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}

	return backfillManifest(filepath.Dir(path))
}

// backfillManifest scans dir for downlink-digest-*.html files and produces a
// manifest with a best-effort entry per file. Returns an empty manifest when
// the directory itself is missing.
func backfillManifest(dir string) (Manifest, error) {
	m := Manifest{Version: ManifestVersion}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil
		}
		return Manifest{}, fmt.Errorf("scan output dir for backfill: %w", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "index.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		if !strings.HasPrefix(name, "downlink-digest-") {
			continue
		}
		m.Digests = append(m.Digests, ManifestEntry{
			Filename:    name,
			DisplayDate: filenameToDisplayDate(name),
		})
	}
	sortDigestsNewestFirst(m.Digests)
	return m, nil
}

// Upsert inserts entry or replaces an existing entry with the same Id, then
// re-sorts newest-first by Filename. Empty Ids never collide so backfilled
// rows (Id == "") are preserved when a real entry with the same filename is
// added — the real one wins by filename match.
func (m *Manifest) Upsert(entry ManifestEntry) {
	for i, existing := range m.Digests {
		if (entry.Id != "" && existing.Id == entry.Id) || existing.Filename == entry.Filename {
			m.Digests[i] = entry
			sortDigestsNewestFirst(m.Digests)
			return
		}
	}
	m.Digests = append(m.Digests, entry)
	sortDigestsNewestFirst(m.Digests)
}

// Write atomically serializes the manifest to path.
func (m Manifest) Write(path string) error {
	if m.Version == 0 {
		m.Version = ManifestVersion
	}
	m.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create manifest dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "manifest-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp manifest: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp manifest: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename manifest: %w", err)
	}
	return nil
}

// ArticleSetHash returns a stable sha256 (hex) over the digest's article ids.
// Two digests built from exactly the same article set produce the same hash.
func ArticleSetHash(d models.Digest) string {
	ids := make([]string, len(d.Articles))
	for i, a := range d.Articles {
		ids[i] = a.Id
	}
	sort.Strings(ids)
	h := sha256.New()
	for _, id := range ids {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ManifestEntryFromDigest builds a ManifestEntry for the given digest using
// the same filename and provider/model labelling logic the publisher uses.
func ManifestEntryFromDigest(d models.Digest) ManifestEntry {
	providerType, modelName := digestProviderLabel(d)
	return ManifestEntry{
		Id:             d.Id,
		Filename:       DigestHTMLFilename(d),
		DisplayDate:    d.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"),
		ProviderType:   providerType,
		ModelName:      modelName,
		ArticleSetHash: ArticleSetHash(d),
	}
}

// digestProviderLabel picks a provider/model label for a digest. Falls back
// to "unknown" when no provider results have been recorded yet.
func digestProviderLabel(d models.Digest) (providerType, modelName string) {
	for _, r := range d.ProviderResults {
		if r.ProviderType != "" {
			return r.ProviderType, r.ModelName
		}
	}
	return "unknown", "unknown"
}

// filenameToDisplayDate extracts a human-readable date from a digest filename.
// e.g. "downlink-digest-2026-04-24_1200.html" → "2026-04-24 12:00 UTC"
func filenameToDisplayDate(filename string) string {
	name := strings.TrimSuffix(filename, ".html")
	const prefix = "downlink-digest-"
	if !strings.HasPrefix(name, prefix) {
		return filename
	}
	datePart := strings.TrimPrefix(name, prefix)
	t, err := time.Parse("2006-01-02_1504", datePart)
	if err != nil {
		return filename
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

// sortDigestsNewestFirst sorts entries by filename desc — ISO timestamp prefix
// sorts lexicographically, matching newest-first chronological order.
func sortDigestsNewestFirst(entries []ManifestEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].Filename > entries[j].Filename
	})
}
