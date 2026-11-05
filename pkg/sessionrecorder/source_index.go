package sessionrecorder

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

type sourceIndex struct {
	db *sql.DB
}

func openSourceIndex(workDir string) (*sourceIndex, error) {
	if err := os.MkdirAll(terminalManifestDir(workDir), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", filepath.Join(manifestDir(workDir), "source-index.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL;
		PRAGMA foreign_keys=ON;
		CREATE TABLE IF NOT EXISTS terminal_batches (
			batch_id TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			expires_at INTEGER
		);
		CREATE TABLE IF NOT EXISTS batch_sources (
			batch_id TEXT NOT NULL REFERENCES terminal_batches(batch_id) ON DELETE CASCADE,
			identity BLOB NOT NULL,
			PRIMARY KEY(batch_id, identity)
		);
		CREATE INDEX IF NOT EXISTS batch_sources_identity ON batch_sources(identity);
		CREATE INDEX IF NOT EXISTS terminal_batches_expires_at ON terminal_batches(expires_at);`); err != nil {
		db.Close()
		return nil, err
	}
	return &sourceIndex{db: db}, nil
}

func (i *sourceIndex) close() { _ = i.db.Close() }

func (i *sourceIndex) contains(identity [16]byte, now time.Time) (bool, error) {
	var found int
	err := i.db.QueryRow(`SELECT 1 FROM batch_sources s JOIN terminal_batches b USING(batch_id)
		WHERE s.identity = ? AND (b.expires_at IS NULL OR b.expires_at > ?) LIMIT 1`, identity[:], now.UnixNano()).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (i *sourceIndex) add(m BatchManifest) error {
	updated, err := time.Parse(time.RFC3339Nano, m.UpdatedAt)
	if err != nil {
		updated = time.Time{}
	}
	var expires any
	if m.State == StateCleaned && !updated.IsZero() {
		expires = updated.Add(cleanedManifestRetention).UnixNano()
	}
	tx, err := i.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO terminal_batches(batch_id, state, updated_at, expires_at) VALUES(?, ?, ?, ?)`, m.BatchID, m.State, updated.UnixNano(), expires); err != nil {
		return err
	}
	for _, source := range m.Sources {
		identity := sourceIdentity(source)
		if _, err := tx.Exec(`INSERT OR IGNORE INTO batch_sources(batch_id, identity) VALUES(?, ?)`, m.BatchID, identity[:]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (i *sourceIndex) moveManifest(workDir string, m BatchManifest) error {
	source := manifestPath(workDir, m.BatchID)
	target := terminalManifestPath(workDir, m.BatchID)
	if err := os.Rename(source, target); err != nil {
		if os.IsNotExist(err) {
			if _, statErr := os.Stat(target); statErr == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

func (i *sourceIndex) expireCleaned(workDir string, now time.Time) error {
	for {
		rows, err := i.db.Query(`SELECT batch_id FROM terminal_batches WHERE expires_at IS NOT NULL AND expires_at <= ? ORDER BY batch_id LIMIT 256`, now.UnixNano())
		if err != nil {
			return err
		}
		var expired []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			expired = append(expired, id)
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if len(expired) == 0 {
			return nil
		}
		for _, id := range expired {
			if err := os.Remove(terminalManifestPath(workDir, id)); err != nil && !os.IsNotExist(err) {
				return err
			}
			tx, err := i.db.Begin()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`DELETE FROM batch_sources WHERE batch_id = ?`, id); err != nil {
				tx.Rollback()
				return err
			}
			if _, err := tx.Exec(`DELETE FROM terminal_batches WHERE batch_id = ?`, id); err != nil {
				tx.Rollback()
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
		}
	}
}
