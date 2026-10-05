package store

import (
	"fmt"
	"io"
	"os"
)

// Backup writes a consistent snapshot of the database to w, using
// SQLite's VACUUM INTO. It stages the snapshot in a temp file (VACUUM
// INTO requires a path that doesn't already exist) and removes that
// file once it has been streamed out.
func (s *Store) Backup(w io.Writer) error {
	tmp, err := os.CreateTemp("", "shop-backup-*.db")
	if err != nil {
		return fmt.Errorf("backup: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	tmp.Close()
	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("backup: remove placeholder: %w", err)
	}
	defer os.Remove(tmpPath) //nolint:errcheck // best effort cleanup

	if _, err := s.db.Exec(`VACUUM INTO ?`, tmpPath); err != nil {
		return fmt.Errorf("backup: vacuum into: %w", err)
	}

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
