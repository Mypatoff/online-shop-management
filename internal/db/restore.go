package db

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Restoring replaces the database file, which is open while the app runs, so
// it is done in two steps: StageRestore validates a backup and puts a copy
// next to the database as "<db>.restore", and the next startup swaps it in
// (ApplyPendingRestore) before the database is opened. The caller takes a
// safety backup of the current database before staging.

// ErrInvalidBackup is wrapped by every error that says a file cannot be
// restored because of what it contains (as opposed to an I/O failure), so
// callers can show it to the user as-is.
var ErrInvalidBackup = errors.New("not a usable ShopKeeper backup")

const sqliteMagic = "SQLite format 3\x00"

func restorePath(dbPath string) string { return dbPath + ".restore" }

// PreRestoreBackupName is the name of the safety backup taken just before a
// restore replaces the database.
func PreRestoreBackupName(t time.Time) string {
	return fmt.Sprintf("shop-prerestore-%s.db", t.Format("20060102-150405"))
}

// BackupFile is one restorable file in the backup folder.
type BackupFile struct {
	Name    string
	Size    int64
	ModTime time.Time
}

// ListBackups returns the .db files in dir, newest first. Half-written
// temporary files and subfolders are skipped. A missing dir is an empty list.
func ListBackups(dir string) ([]BackupFile, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []BackupFile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	files := []BackupFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, BackupFile{Name: e.Name(), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].ModTime.Equal(files[j].ModTime) {
			return files[i].ModTime.After(files[j].ModTime)
		}
		return files[i].Name > files[j].Name
	})
	return files, nil
}

// BackupPath resolves a file name from ListBackups to a path inside dir. It
// rejects anything that is not a plain file name, so a request cannot reach
// outside the backup folder.
func BackupPath(dir, name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) ||
		!strings.HasSuffix(name, ".db") {
		return "", fmt.Errorf("%w: bad file name", ErrInvalidBackup)
	}
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: %s does not exist", ErrInvalidBackup, name)
		}
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not a file", ErrInvalidBackup, name)
	}
	return path, nil
}

// ValidateBackup checks that the SQLite file at path can replace the live
// database: it is a SQLite file, passes PRAGMA quick_check, has the
// products and sales tables, and its schema version is not newer than this
// build understands (an older one is fine, it is migrated on the next start).
// It opens path read-write, so pass a private copy, never a live file.
func ValidateBackup(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	head := make([]byte, len(sqliteMagic))
	_, err = io.ReadFull(f, head)
	f.Close()
	if err != nil || !bytes.Equal(head, []byte(sqliteMagic)) {
		return fmt.Errorf("%w: not a SQLite database file", ErrInvalidBackup)
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)

	var result string
	if err := conn.QueryRow(`PRAGMA quick_check`).Scan(&result); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: the file is damaged (%s)", ErrInvalidBackup, result)
	}

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	if version > CurrentVersion {
		return fmt.Errorf("%w: it was made by a newer ShopKeeper (schema %d, this version supports %d); update the app first",
			ErrInvalidBackup, version, CurrentVersion)
	}

	for _, table := range []string{"products", "sales"} {
		ok, err := tableExists(conn, table)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidBackup, err)
		}
		if !ok {
			return fmt.Errorf("%w: it has no %q table", ErrInvalidBackup, table)
		}
	}
	return nil
}

// StageRestore copies src to "<dbPath>.restore" (leaving src untouched),
// validating the copy first. Nothing the app uses changes until the next
// start; a copy that fails validation is removed.
func StageRestore(dbPath, src string) error {
	staged := restorePath(dbPath)
	tmp := staged + ".tmp"
	defer os.Remove(tmp) //nolint:errcheck // gone after the rename below

	if err := copyFile(src, tmp); err != nil {
		return fmt.Errorf("stage restore: %w", err)
	}
	if err := ValidateBackup(tmp); err != nil {
		removeSQLiteSidecars(tmp)
		return err
	}
	removeSQLiteSidecars(tmp)

	_ = os.Remove(staged)
	if err := os.Rename(tmp, staged); err != nil {
		return fmt.Errorf("stage restore: %w", err)
	}
	return nil
}

// CancelRestore removes a staged restore that has not been applied yet.
func CancelRestore(dbPath string) {
	_ = os.Remove(restorePath(dbPath))
}

// ApplyPendingRestore swaps in a restore staged by StageRestore, if there is
// one. It must run before the database is opened. The old database's -wal and
// -shm files go with it: a leftover WAL from the old file would be replayed
// onto the restored one.
func ApplyPendingRestore(dbPath string) (applied bool, err error) {
	staged := restorePath(dbPath)
	if _, err := os.Stat(staged); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	removeSQLiteSidecars(dbPath)
	// Windows cannot rename over an existing file. The staged copy stays in
	// place until the rename succeeds, so a crash in between retries cleanly
	// on the next start.
	if err := os.Remove(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("restore: remove old database: %w", err)
	}
	if err := os.Rename(staged, dbPath); err != nil {
		return false, fmt.Errorf("restore: move restored database into place: %w", err)
	}
	return true, nil
}

func removeSQLiteSidecars(path string) {
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	_ = os.Remove(path + "-journal")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
