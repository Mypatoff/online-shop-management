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
)

func main() {
	if err := run(); err != nil {
		reportFatal(err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// The app shows its own window (Windows) unless NO_BROWSER is set; with
	// no console, its log goes to a file.
	windowMode := windowedUI && !noBrowser()
	if windowMode {
		setupLogging(filepath.Dir(cfg.DBPath))
	}

	if err := os.MkdirAll(cfg.BackupDir, 0o755); err != nil {
		return fmt.Errorf("backup dir: %w", err)
	}
	absBackupDir, err := filepath.Abs(cfg.BackupDir)
	if err != nil {
		absBackupDir = cfg.BackupDir
	}
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
	a := api.New(s, cfg.ShopName, cfg.Currency, cfg.Decimals)
	h, err := handlers.New()
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
		if errors.Is(err, syscall.EADDRINUSE) {
			close(backupStop)
			<-backupDone
			conn.Close()

			if alreadyRunning(cfg.Port) {
				// Bring up the running copy's window; failing that, its page.
				if !(windowMode && focusExisting()) {
					openBrowser(fmt.Sprintf("http://localhost:%d", cfg.Port))
				}
				log.Printf("ShopKeeper is already running on port %d", cfg.Port)
				os.Exit(0)
			}
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
	if !windowMode {
		openBrowser(url)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
