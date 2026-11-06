// Command server runs the ShopKeeper API.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"shop/internal/api"
	"shop/internal/config"
	"shop/internal/db"
	"shop/internal/handlers"
	"shop/internal/store"
	"shop/internal/update"
)

// updater checks GitHub for new releases and replaces this program with one. It is created in
// run and used again in main, after run has closed the database.
var updater = update.New(update.Version)

// errAlreadyRunning means a second launch found the first one and woke it up.
var errAlreadyRunning = errors.New("already running")

// restartedEnv marks a process started again by an update or a restore, so on Linux it does
// not open a second browser tab (the open page reconnects by itself).
const restartedEnv = "SHOPKEEPER_RESTARTED"

// restoreRestart is set when the app stops only to start again after a restore.
var restoreRestart atomic.Bool

func main() {
	err := run()
	if errors.Is(err, errAlreadyRunning) {
		return
	}
	// An update is installed only now: the server has stopped, the last backup is taken and the
	// database is closed, so the new program starts from a clean state.
	if updater.Pending() {
		finishUpdate()
	} else if restoreRestart.Load() && err == nil {
		// The restored database is swapped in by the next start (see db.ApplyPendingRestore).
		_ = os.Setenv(restartedEnv, "1")
		if err := updater.Relaunch(); err != nil {
			reportFatal(fmt.Errorf("the backup is ready to restore but ShopKeeper could not be restarted: %w\n\nStart it again by hand", err))
			os.Exit(1)
		}
	}
	if err != nil {
		reportFatal(err)
		os.Exit(1)
	}
}

// finishUpdate swaps in the downloaded program and starts it. If the swap fails the old
// program is still in place and is started again, so the shop is never left without an app.
func finishUpdate() {
	if err := updater.Install(); err != nil {
		log.Printf("update failed, keeping the current version: %v", err)
	} else {
		log.Printf("updated to the new version")
	}
	_ = os.Setenv(restartedEnv, "1")
	if err := updater.Relaunch(); err != nil {
		log.Printf("could not restart ShopKeeper: %v", err)
		reportFatal(fmt.Errorf("the update is installed but ShopKeeper could not be restarted: %w\n\nStart it again by hand", err))
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// stopApp ends the main wait below; requestStop also says whether the app should start
	// again afterwards (a restore) and whether to take the usual final backup on the way out.
	stopCtx, stopApp := context.WithCancel(context.Background())
	defer stopApp()
	finalBackup := true
	requestStop := func(restartAfter, backup bool) {
		finalBackup = backup
		restoreRestart.Store(restartAfter)
		stopApp()
	}

	// The app shows its own window (Windows) unless NO_BROWSER is set; with
	// no console, its log goes to a file. Elsewhere setupLogging only does so
	// if there is no terminal to log to.
	windowMode := windowedUI && !noBrowser()
	if windowMode || !windowedUI {
		setupLogging(filepath.Dir(cfg.DBPath))
	}

	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		return fmt.Errorf("backup dir: %w", err)
	}
	absBackupDir, err := filepath.Abs(cfg.BackupDir)
	if err != nil {
		absBackupDir = cfg.BackupDir
	}
	log.Printf("ShopKeeper %s", update.Version)
	updater.CleanupLeftovers()
	log.Printf("backups: %s", absBackupDir)

	// A restore chosen in the app last time is applied now, before the database file is
	// opened. If it cannot be, say so rather than silently starting on the old data and
	// letting the user believe it was restored.
	if applied, err := db.ApplyPendingRestore(cfg.DBPath); err != nil {
		return fmt.Errorf("applying the backup restore: %w", err)
	} else if applied {
		log.Printf("restored the database from the chosen backup")
	}

	conn, err := db.Open(cfg.DBPath, cfg.BackupDir)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer conn.Close()

	if needed, err := db.NeedsStartupBackup(cfg.BackupDir, time.Now()); err != nil {
		log.Printf("backup: check today's file: %v", err)
	} else if needed {
		runBackup(conn, cfg.BackupDir, cfg.BackupKeepDays)
	}

	if needed, err := db.NeedsStartBackup(cfg.BackupDir, time.Now()); err != nil {
		log.Printf("backup: check start-of-day file: %v", err)
	} else if needed {
		startPath := filepath.Join(cfg.BackupDir, db.StartBackupName(time.Now()))
		if err := db.Backup(conn, startPath); err != nil {
			log.Printf("START-OF-DAY BACKUP FAILED: %v", err)
		} else {
			log.Printf("start-of-day backup written: %s", startPath)
		}
	}

	backupStop := make(chan struct{})
	backupDone := make(chan struct{})
	go func() {
		defer close(backupDone)
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runBackup(conn, cfg.BackupDir, cfg.BackupKeepDays)
			case <-backupStop:
				return
			}
		}
	}()

	s := store.New(conn)
	a := api.New(s, cfg.ShopName, cfg.Currency, cfg.Decimals).WithUpdater(updater)
	a.SetLang(cfg.Lang)
	maint := api.Maintenance{
		DBPath:    cfg.DBPath,
		BackupDir: cfg.BackupDir,
		// The restore took its own safety backup; another one would overwrite today's daily
		// file, which may be the very backup being restored.
		Restart: func(backup bool) { requestStop(true, backup) },
	}
	if !windowedUI {
		// With a window, closing it quits. In a browser there is nothing to close.
		maint.Quit = func() { requestStop(false, true) }
	}
	a.SetMaintenance(maint)
	h, err := handlers.New(cfg.Lang)
	if err != nil {
		return fmt.Errorf("templates: %w", err)
	}
	pageRoutes, err := h.Routes()
	if err != nil {
		return fmt.Errorf("page routes: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", a.Routes(cfg.Port))
	mux.Handle("/", pageRoutes)

	addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		close(backupStop)
		<-backupDone

		// Don't match a specific errno here: on Windows net.Listen returns the Winsock
		// WSAEADDRINUSE (10048), which is not syscall.EADDRINUSE. Ask the port instead; if
		// our own app answers, this is a second launch.
		if alreadyRunning(cfg.Port) {
			conn.Close()
			// Bring up the running copy's window; failing that, its page.
			if !(windowMode && focusExisting()) {
				openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
			}
			log.Printf("ShopKeeper is already running on port %d", cfg.Port)
			return errAlreadyRunning
		}
		if isAddrInUse(err) {
			return fmt.Errorf("port %d is used by another program; set PORT=... to use another", cfg.Port)
		}
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second, // backups can take a little longer
		IdleTimeout:       60 * time.Second,
	}

	absDB, err := filepath.Abs(cfg.DBPath)
	if err != nil {
		absDB = cfg.DBPath
	}
	log.Printf("ShopKeeper listening on http://%s", addr)
	log.Printf("database: %s", absDB)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(listener) }()
	url := fmt.Sprintf("http://localhost:%d", cfg.Port)
	if !windowMode && os.Getenv(restartedEnv) != "1" {
		openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(stopCtx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Wait for whatever ends the app: the server failing, a stop signal, or
	// (in window mode) the user closing the window.
	var serverErr, uiErr error
	if windowMode {
		quit := make(chan struct{})
		died := make(chan error, 1)
		go func() {
			select {
			case err := <-serveErr:
				died <- err
			case <-ctx.Done():
			case <-updater.Restart():
				log.Print("update downloaded, restarting")
			}
			close(quit) // closes the window
		}()
		uiErr = showWindow(url, quit) // returns when the window is gone
		select {
		case serverErr = <-died:
		default:
		}
	} else {
		select {
		case serverErr = <-serveErr:
		case <-ctx.Done():
		case <-updater.Restart():
			log.Print("update downloaded, restarting")
		}
	}

	if serverErr != nil {
		close(backupStop)
		if !errors.Is(serverErr, http.ErrServerClosed) {
			return fmt.Errorf("server: %w", serverErr)
		}
		return nil
	}

	log.Print("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := srv.Shutdown(shutdownCtx)
	close(backupStop)
	<-backupDone
	// Final backup after the server has stopped taking requests, but
	// before conn.Close() runs via the deferred call above.
	if finalBackup {
		runBackup(conn, cfg.BackupDir, cfg.BackupKeepDays)
	}
	if shutdownErr != nil {
		return fmt.Errorf("shutdown: %w", shutdownErr)
	}
	if uiErr != nil {
		return uiErr
	}
	return nil
}

// runBackup takes a daily backup of conn into backupDir and rotates out
// backups older than keepDays. Failures are logged, never fatal — a
// missed backup shouldn't take down the server.
func runBackup(conn *sql.DB, backupDir string, keepDays int) {
	dest := filepath.Join(backupDir, db.DailyBackupName(time.Now()))
	if err := db.Backup(conn, dest); err != nil {
		log.Printf("BACKUP FAILED: %v", err)
		return
	}
	log.Printf("backup written: %s", dest)
	if err := db.Rotate(backupDir, keepDays); err != nil {
		log.Printf("backup rotation failed: %v", err)
	}
}
