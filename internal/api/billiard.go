package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"shop/internal/store"
)

type billiardEntryDTO struct {
	ID        int64  `json:"id"`
	Amount    int64  `json:"amount"`
	Table     string `json:"table"`
	Minutes   *int64 `json:"minutes"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
	Voided    bool   `json:"voided"`
}

func toBilliardEntryDTO(e store.BilliardEntry) billiardEntryDTO {
	return billiardEntryDTO{
		ID: e.ID, Amount: e.Amount, Table: e.TableName, Minutes: e.Minutes,
		Note: e.Note, CreatedAt: e.CreatedAt, Voided: e.Voided(),
	}
}

type createBilliardRequest struct {
	Amount  int64  `json:"amount"`
	Table   string `json:"table"`
	Minutes *int64 `json:"minutes"`
	Note    string `json:"note"`
}

// handleCreateBilliardEntry records one payment for table time. There
// is no stock involved.
func (a *API) handleCreateBilliardEntry(w http.ResponseWriter, r *http.Request) {
	var req createBilliardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if err := validateBilliardAmount(req.Amount); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	table, err := validateTableName(req.Table)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMinutes(req.Minutes); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	note, err := validateNote(req.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	entry, err := a.store.CreateBilliardEntry(req.Amount, table, req.Minutes, note)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not record billiard entry")
		return
	}
	writeJSON(w, http.StatusCreated, toBilliardEntryDTO(entry))
}

func (a *API) handleVoidBilliardEntry(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid billiard entry id")
		return
	}

	err = a.store.VoidBilliardEntry(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "billiard entry not found")
	case errors.Is(err, store.ErrAlreadyVoided):
		writeError(w, http.StatusConflict, "billiard entry already voided")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not void billiard entry")
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"voided": true})
	}
}

type billiardTotalsDTO struct {
	Revenue int64 `json:"revenue"`
	Count   int64 `json:"count"`
}

type billiardDailyDTO struct {
	Date    string             `json:"date"`
	Entries []billiardEntryDTO `json:"entries"`
	Totals  billiardTotalsDTO  `json:"totals"`
	Tables  []string           `json:"tables"`
}

// handleListBilliard reports one local calendar day's billiard
// entries (oldest first, voided included), totals over the
// non-voided ones, and up to 10 recently-used table names (across all
// time, not just this day) for the entry form's suggestions.
func (a *API) handleListBilliard(w http.ResponseWriter, r *http.Request) {
	day := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		parsed, err := time.ParseInLocation(dailyDateLayout, v, time.Local)
		if err != nil {
			writeError(w, http.StatusBadRequest, "date must be in YYYY-MM-DD format")
			return
		}
		day = parsed
	}

	entries, err := a.store.DailyBilliard(day)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load billiard entries")
		return
	}
	tables, err := a.store.RecentBilliardTables(10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load billiard entries")
		return
	}

	resp := billiardDailyDTO{
		Date:    day.Format(dailyDateLayout),
		Entries: make([]billiardEntryDTO, 0, len(entries)),
		Tables:  tables,
	}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, toBilliardEntryDTO(e))
		if !e.Voided() {
			resp.Totals.Revenue += e.Amount
			resp.Totals.Count++
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
