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

func main() {
	err := run()
	// An update is installed only now: the server has stopped, the last backup is taken and the
	// database is closed, so the new program starts from a clean state.
	if updater.Pending() {
		finishUpdate()
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

	// On Windows the app has no console, so its log goes to a file. By default it shows its
	// own native window and runs no web server at all. SHOPKEEPER_UI=web serves the pages and
	// opens the browser instead (as on Linux); NO_BROWSER=1 runs the server with no UI.
	if windowedUI {
		setupLogging(filepath.Dir(cfg.DBPath))
	}
	native := windowedUI && !noBrowser() && os.Getenv("SHOPKEEPER_UI") != "web"
	if native {
		absDB, err := filepath.Abs(cfg.DBPath)
		if err != nil {
			absDB = cfg.DBPath
		}
		if !acquireSingleInstance(absDB) {
			focusExisting() // bring the first window to the front
			log.Print("ShopKeeper is already running")
			os.Exit(0)
		}
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
	apiHandler := a.Routes(cfg.Port)

	absDB, err := filepath.Abs(cfg.DBPath)
	if err != nil {
		absDB = cfg.DBPath
	}
	log.Printf("database: %s", absDB)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Browser mode: serve the web pages and the API on localhost.
	var srv *http.Server
	serveErr := make(chan error, 1)
	if !native {
		h, err := handlers.New()
		if err != nil {
			return fmt.Errorf("templates: %w", err)
		}
		pageRoutes, err := h.Routes()
		if err != nil {
			return fmt.Errorf("page routes: %w", err)
		}

		mux := http.NewServeMux()
		mux.Handle("/api/", apiHandler)
		mux.Handle("/", pageRoutes)

		addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			if errors.Is(err, syscall.EADDRINUSE) {
				close(backupStop)
				<-backupDone
				conn.Close()

				if alreadyRunning(cfg.Port) {
					// Show the running copy's page.
					openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
					log.Printf("ShopKeeper is already running on port %d", cfg.Port)
					os.Exit(0)
				}
				return fmt.Errorf("port %d is used by another program; set PORT=... to use another", cfg.Port)
			}
			return fmt.Errorf("listen on %s: %w", addr, err)
		}

		srv = &http.Server{
			Handler:           mux,
			ReadTimeout:       10 * time.Second,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      30 * time.Second, // backups can take a little longer
			IdleTimeout:       60 * time.Second,
		}
		log.Printf("ShopKeeper listening on http://%s", addr)
		go func() { serveErr <- srv.Serve(listener) }()
		openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
	}

	// Wait for whatever ends the app: the server failing, a stop signal, an update that is
	// ready to install, or (native mode) the user closing the window.
	var serverErr, uiErr error
	if native {
		quit := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
			case <-updater.Restart():
				log.Print("update downloaded, restarting")
			}
			close(quit) // closes the window
		}()
		uiErr = runNativeUI(apiHandler, cfg.Port, quit) // returns when the window is gone
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
	var shutdownErr error
	if srv != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		shutdownErr = srv.Shutdown(shutdownCtx)
	}
	close(backupStop)
	<-backupDone
	// Final backup after the server has stopped taking requests, but
	// before conn.Close() runs via the deferred call above.
	runBackup(conn, cfg.BackupDir, cfg.BackupKeepDays)
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
