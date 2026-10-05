package api

import (
	"fmt"
	"log"
	"net/http"
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

func (a *API) handleBackup(w http.ResponseWriter, r *http.Request) {
	filename := fmt.Sprintf("shop-backup-%s.db", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	if err := a.store.Backup(w); err != nil {
		// Headers (and maybe part of the body) are already sent, so we
		// can't change the status code at this point; just log it.
		log.Printf("backup failed: %v", err)
	}
}
