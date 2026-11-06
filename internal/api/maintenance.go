package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"shop/internal/db"
)

const (
	restoreUploadPath = "/api/restore/upload"
	// maxUploadBytes caps an uploaded backup. A shop's database is a few
	// megabytes; this only stops a runaway upload from filling the disk.
	maxUploadBytes = 1 << 30
	// uploadTimeout replaces the server's short read timeout for an upload,
	// which must also work from a slow disk.
	uploadTimeout = 10 * time.Minute
)

// Maintenance is what the restore and quit endpoints need from the
// running program. Anything left nil disables that feature.
type Maintenance struct {
	DBPath    string
	BackupDir string
	// Restart stops the app and starts it again. finalBackup says whether to
	// take the usual backup on the way out.
	Restart func(finalBackup bool)
	// Quit stops the app. It is nil where the app has its own window, since
	// closing the window is how it is stopped there.
	Quit func()
}

// SetMaintenance wires in the program-level hooks.
func (a *API) SetMaintenance(m Maintenance) { a.maint = m }

// afterResponse runs f once the response has been sent, so the browser gets
// its answer before the app goes away.
func afterResponse(f func()) {
	go func() {
		time.Sleep(500 * time.Millisecond)
		f()
	}()
}

type backupFileDTO struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified"`
}

func (a *API) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if a.maint.BackupDir == "" {
		writeJSON(w, http.StatusOK, map[string]any{"backups": []backupFileDTO{}})
		return
	}
	files, err := db.ListBackups(a.maint.BackupDir)
	if err != nil {
		log.Printf("list backups: %v", err)
		writeError(w, http.StatusInternalServerError, "could not list backups")
		return
	}
	out := make([]backupFileDTO, 0, len(files))
	for _, f := range files {
		out = append(out, backupFileDTO{Name: f.Name, Size: f.Size, Modified: f.ModTime.Unix()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": out})
}

// restoreFrom validates the file at src, takes a safety backup of the current
// database, and stages src to replace it at the next start. On success it
// answers and schedules the restart.
func (a *API) restoreFrom(w http.ResponseWriter, src string) {
	if a.maint.DBPath == "" || a.maint.Restart == nil {
		writeError(w, http.StatusNotImplemented, "restore is not available")
		return
	}
	if err := db.StageRestore(a.maint.DBPath, src); err != nil {
		if errors.Is(err, db.ErrInvalidBackup) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Printf("restore: stage: %v", err)
		writeError(w, http.StatusInternalServerError, "could not prepare the restore; nothing was changed")
		return
	}

	safety := filepath.Join(a.maint.BackupDir, db.PreRestoreBackupName(time.Now()))
	if err := a.store.BackupTo(safety); err != nil {
		db.CancelRestore(a.maint.DBPath)
		log.Printf("restore: safety backup: %v", err)
		writeError(w, http.StatusInternalServerError, "could not back up the current data first, so nothing was changed")
		return
	}
	log.Printf("restore: current data saved to %s, restoring from %s", safety, filepath.Base(src))

	writeJSON(w, http.StatusOK, map[string]any{"restarting": true, "safety_backup": filepath.Base(safety)})
	// The safety backup is the final backup; another one would overwrite
	// today's daily file, which may be the very backup being restored.
	afterResponse(func() { a.maint.Restart(false) })
}

func (a *API) handleRestore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	path, err := db.BackupPath(a.maint.BackupDir, req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	a.restoreFrom(w, path)
}

// handleRestoreUpload restores from a database file sent in the request body
// (a backup downloaded earlier).
func (a *API) handleRestoreUpload(w http.ResponseWriter, r *http.Request) {
	if a.maint.BackupDir == "" {
		writeError(w, http.StatusNotImplemented, "restore is not available")
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(uploadTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(uploadTimeout))

	if err := os.MkdirAll(a.maint.BackupDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the upload")
		return
	}
	tmp, err := os.CreateTemp(a.maint.BackupDir, "upload-*.tmp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not store the upload")
		return
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // the staged copy is what is kept
	_, copyErr := io.Copy(tmp, r.Body)
	closeErr := tmp.Close()
	if copyErr != nil {
		var tooBig *http.MaxBytesError
		if errors.As(copyErr, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "that file is too large to be a backup")
			return
		}
		writeError(w, http.StatusBadRequest, "the upload was interrupted")
		return
	}
	if closeErr != nil {
		writeError(w, http.StatusInternalServerError, "could not store the upload")
		return
	}
	a.restoreFrom(w, tmp.Name())
}

func (a *API) handleQuit(w http.ResponseWriter, r *http.Request) {
	if a.maint.Quit == nil {
		writeError(w, http.StatusNotImplemented, "this app is stopped by closing its window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"quitting": true})
	afterResponse(a.maint.Quit)
}
