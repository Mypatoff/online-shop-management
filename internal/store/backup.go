package store

import (
	"fmt"
	"io"
	"os"
)

// BackupToTempFile creates a full snapshot of the database in a new
// temp file, using SQLite's VACUUM INTO, and returns its path. The
// caller owns the file and is responsible for removing it once done.
// Creating the whole snapshot up front - before any bytes reach an
// HTTP response - lets a caller like handleBackup fail with a clean
// JSON error instead of a half-written download.
func (s *Store) BackupToTempFile() (string, error) {
	tmp, err := os.CreateTemp("", "shop-backup-*.db")
	if err != nil {
		return "", fmt.Errorf("backup: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	if err := os.Remove(tmpPath); err != nil {
		return "", fmt.Errorf("backup: remove placeholder: %w", err)
	}

	if _, err := s.db.Exec(`VACUUM INTO ?`, tmpPath); err != nil {
		return "", fmt.Errorf("backup: vacuum into: %w", err)
	}
	return tmpPath, nil
}

// Backup writes a consistent snapshot of the database to w; see
// BackupToTempFile.
func (s *Store) Backup(w io.Writer) error {
	tmpPath, err := s.BackupToTempFile()
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath) //nolint:errcheck // best effort cleanup

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
