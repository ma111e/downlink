package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"

	"github.com/ma111e/downlink/cmd/server/internal/config"

	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// Magic-byte prefixes used to detect the current encoding of a blob so the
// migration is idempotent: already-zstd blobs are left alone, gzip blobs are
// transcoded, and anything else is treated as plaintext to compress.
var (
	gzipMagic = []byte{0x1f, 0x8b}
	zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// blobColumns are the (table, column) pairs the app now stores zstd-compressed.
// The first group was gzip-compressed before; the second was plaintext.
var blobColumns = []struct{ table, col string }{
	{"feed_refresh_results", "raw_body"},
	{"feed_refresh_results", "error_log"},
	{"feed_refresh_results", "warning_log"},
	{"llm_calls", "prompt"},
	{"llm_calls", "response"},
	{"article_analyses", "raw_response"},
	{"article_analyses", "comprehensive_synthesis"},
	{"article_analyses", "standard_synthesis"},
	{"article_analyses", "brief_overview"},
	{"article_analyses", "thinking_process"},
}

// newMigrateCommand converts an existing DB to the current storage format:
// re-encodes every blob column to zstd, drops the unmaintained tags.use_count,
// deletes unused tags, and VACUUMs into incremental auto-vacuum mode. It is safe
// to re-run (each step is guarded/idempotent). It opens the file with a raw
// sql.DB so no schema auto-migration or serializer runs mid-conversion.
func newMigrateCommand() *cobra.Command {
	var dbPath string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "One-time storage migration: zstd blobs, drop tags.use_count, GC tags, VACUUM",
		// Override the root PersistentPreRunE so migrate stays self-contained: it
		// must not open/auto-migrate the live DB, apply profiles, or start the feed
		// manager. It operates on the file directly via a raw sql.DB.
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error { return nil },
		RunE: func(_ *cobra.Command, _ []string) error {
			path := dbPath
			if path == "" {
				if err := config.Init(); err != nil {
					return fmt.Errorf("load config: %w", err)
				}
				path = config.Config.DbPath
			}
			log.WithField("db", path).Info("starting storage migration")

			db, err := sql.Open("sqlite3", path)
			if err != nil {
				return fmt.Errorf("open db: %w", err)
			}
			defer db.Close()
			db.SetMaxOpenConns(1)

			enc, err := zstd.NewWriter(nil)
			if err != nil {
				return fmt.Errorf("zstd writer: %w", err)
			}
			defer enc.Close()
			dec, err := zstd.NewReader(nil)
			if err != nil {
				return fmt.Errorf("zstd reader: %w", err)
			}
			defer dec.Close()

			for _, bc := range blobColumns {
				n, err := recompressColumn(db, enc, dec, bc.table, bc.col)
				if err != nil {
					return fmt.Errorf("recompress %s.%s: %w", bc.table, bc.col, err)
				}
				log.WithFields(log.Fields{"table": bc.table, "column": bc.col, "rewritten": n}).Info("recompressed blob column")
			}

			if err := dropUseCount(db); err != nil {
				return fmt.Errorf("drop tags.use_count: %w", err)
			}

			res, err := db.Exec(`DELETE FROM tags WHERE id NOT IN (SELECT tag_id FROM article_tags)`)
			if err != nil {
				return fmt.Errorf("delete unused tags: %w", err)
			}
			if n, _ := res.RowsAffected(); true {
				log.WithField("deleted", n).Info("removed unused tags")
			}

			// Switch the file into full auto-vacuum (shrinks automatically on later
			// commits) and reclaim every free page now. VACUUM is what makes the
			// auto_vacuum change take effect.
			if _, err := db.Exec(`PRAGMA auto_vacuum = FULL`); err != nil {
				return fmt.Errorf("set auto_vacuum: %w", err)
			}
			log.Info("running VACUUM (this reclaims free pages and may take a while)")
			if _, err := db.Exec(`VACUUM`); err != nil {
				return fmt.Errorf("vacuum: %w", err)
			}

			log.Info("storage migration complete")
			return nil
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "Path to the SQLite DB to migrate (default: db_path from config.json)")
	return cmd
}

// recompressColumn rewrites every non-empty value of table.col to zstd, skipping
// values already in zstd form. Returns the number of rows rewritten.
func recompressColumn(db *sql.DB, enc *zstd.Encoder, dec *zstd.Decoder, table, col string) (int, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT rowid, %q FROM %q WHERE %q IS NOT NULL AND length(%q) > 0`, col, table, col, col))
	if err != nil {
		return 0, err
	}

	type pending struct {
		rowid int64
		data  []byte
	}
	var updates []pending
	for rows.Next() {
		var rowid int64
		var blob []byte
		if err := rows.Scan(&rowid, &blob); err != nil {
			rows.Close()
			return 0, err
		}
		if bytes.HasPrefix(blob, zstdMagic) {
			continue // already migrated
		}
		var plain []byte
		if bytes.HasPrefix(blob, gzipMagic) {
			plain, err = gunzip(blob)
			if err != nil {
				rows.Close()
				return 0, fmt.Errorf("gunzip rowid %d: %w", rowid, err)
			}
		} else {
			plain = blob // plaintext column
		}
		updates = append(updates, pending{rowid: rowid, data: enc.EncodeAll(plain, nil)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()

	if len(updates) == 0 {
		return 0, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(fmt.Sprintf(`UPDATE %q SET %q = ? WHERE rowid = ?`, table, col))
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	for _, u := range updates {
		if _, err := stmt.Exec(u.data, u.rowid); err != nil {
			_ = stmt.Close()
			_ = tx.Rollback()
			return 0, err
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(updates), nil
}

// dropUseCount removes tags.use_count if present. No-op when already gone.
func dropUseCount(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(tags)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			return err
		}
		if name == "use_count" {
			found = true
		}
	}
	rows.Close()
	if !found {
		log.Info("tags.use_count already absent, skipping drop")
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE tags DROP COLUMN use_count`); err != nil {
		return err
	}
	log.Info("dropped tags.use_count")
	return nil
}

// gunzip decompresses a gzip blob to its plaintext bytes.
func gunzip(b []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
