package store

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"shop/internal/db"
)

// BackupToTempFile creates a full snapshot of the database in a new
// private temp directory, using SQLite's VACUUM INTO, and returns the
// snapshot's path and a cleanup function that removes it. Creating the
// whole snapshot up front - before any bytes reach an HTTP response - lets
// a caller like handleBackup fail with a clean JSON error instead of a
// half-written download.
//
// The snapshot lives in a directory made just for this call (MkdirTemp is
// atomic), so no other process can claim the file name between "pick a
// name" and "VACUUM INTO that name".
func (s *Store) BackupToTempFile() (path string, cleanup func(), err error) {
	dir, err := os.MkdirTemp("", "shop-backup-*")
	if err != nil {
		return "", nil, fmt.Errorf("backup: create temp dir: %w", err)
	}
	cleanup = func() { _ = os.RemoveAll(dir) }

	path = filepath.Join(dir, "snapshot.db")
	if _, err := s.db.Exec(`VACUUM INTO ?`, path); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("backup: vacuum into: %w", err)
	}
	return path, cleanup, nil
}

// Backup writes a consistent snapshot of the database to w; see
// BackupToTempFile.
func (s *Store) Backup(w io.Writer) error {
	tmpPath, cleanup, err := s.BackupToTempFile()
	if err != nil {
		return err
	}
	defer cleanup()

	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("backup: open snapshot: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("backup: stream snapshot: %w", err)
	}
	return nil
}

// BackupTo writes a checked snapshot of the database to destPath (see
// db.Backup): a safety copy for the caller to keep.
func (s *Store) BackupTo(destPath string) error {
	return db.Backup(s.db, destPath)
}
