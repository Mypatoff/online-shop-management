package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"shop_name": a.shopName,
		"currency":  a.currency,
		"decimals":  a.decimals,
	})
}

func (a *API) handleSummary(w http.ResponseWriter, r *http.Request) {
	sum, err := a.store.Summary()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not build summary")
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// handleBackup creates the full snapshot before touching the response
// at all, so a failure can still be reported as a clean JSON 500 with
// no download headers, rather than a half-written attachment.
func (a *API) handleBackup(w http.ResponseWriter, r *http.Request) {
	tmpPath, err := a.store.BackupToTempFile()
	if err != nil {
		log.Printf("backup failed: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create backup")
		return
	}
	defer os.Remove(tmpPath) //nolint:errcheck // best effort cleanup

	f, err := os.Open(tmpPath)
	if err != nil {
		log.Printf("backup failed: open snapshot: %v", err)
		writeError(w, http.StatusInternalServerError, "could not create backup")
		return
	}
	defer f.Close()

	filename := fmt.Sprintf("shop-backup-%s.db", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if _, err := io.Copy(w, f); err != nil {
		// Headers (and maybe part of the body) are already sent, so we
		// can't change the status code at this point; just log it.
		log.Printf("backup stream failed: %v", err)
	}
}
