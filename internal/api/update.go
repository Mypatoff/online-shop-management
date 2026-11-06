package api

import (
	"context"
	"log"
	"net/http"
	"time"
)

func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": a.updater.Current})
}

// handleUpdateCheck asks GitHub whether a newer release exists. It needs the internet, so a
// failure (offline, rate limit) is a 502 with a readable message rather than a server error.
func (a *API) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	info, err := a.updater.Check(ctx)
	if err != nil {
		log.Printf("update check: %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.updater.Status())
}

// handleUpdateApply starts the download and returns at once (202); the page follows the
// progress through /api/update/status. When the file is verified the app shuts down and
// restarts itself (see cmd/server).
func (a *API) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := a.updater.Start(ctx); err != nil {
		log.Printf("update: %v", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	log.Print("update: downloading the new version")
	writeJSON(w, http.StatusAccepted, a.updater.Status())
}
