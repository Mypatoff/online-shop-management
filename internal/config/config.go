// Package config reads server settings from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds everything read from the environment at startup.
type Config struct {
	Port           int
	DBPath         string
	Currency       string
	Decimals       int
	ShopName       string
	BackupDir      string
	BackupKeepDays int
}

// Load reads PORT, DB_PATH, CURRENCY, DECIMALS, SHOP_NAME, BACKUP_DIR
// and BACKUP_KEEP_DAYS from the environment, applying defaults for
// anything unset.
func Load() (Config, error) {
	cfg := Config{
		Port:           8080,
		DBPath:         defaultDBPath(),
		Currency:       "so'm",
		Decimals:       0,
		ShopName:       "ShopKeeper",
		BackupKeepDays: 30,
	}

	if v := os.Getenv("PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			return Config{}, fmt.Errorf("PORT must be an integer 1-65535, got %q", v)
		}
		cfg.Port = port
	}

	if v := os.Getenv("DB_PATH"); v != "" {
		cfg.DBPath = v
	}

	if v := os.Getenv("CURRENCY"); v != "" {
		cfg.Currency = v
	}

	if v := os.Getenv("DECIMALS"); v != "" {
		dec, err := strconv.Atoi(v)
		if err != nil || dec < 0 || dec > 3 {
			return Config{}, fmt.Errorf("DECIMALS must be an integer 0-3, got %q", v)
		}
		cfg.Decimals = dec
	}

	if v := os.Getenv("SHOP_NAME"); v != "" {
		cfg.ShopName = v
	}

	// Default BACKUP_DIR depends on the (possibly overridden) DB_PATH, so
	// it's computed after DB_PATH above but before BACKUP_DIR below.
	cfg.BackupDir = filepath.Join(filepath.Dir(cfg.DBPath), "backups")
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		cfg.BackupDir = v
	}

	if v := os.Getenv("BACKUP_KEEP_DAYS"); v != "" {
		days, err := strconv.Atoi(v)
		if err != nil || days < 1 {
			return Config{}, fmt.Errorf("BACKUP_KEEP_DAYS must be a positive integer, got %q", v)
		}
		cfg.BackupKeepDays = days
	}

	return cfg, nil
}

// defaultDBPath puts shop.db next to the running executable, so a
// double-clicked binary keeps its data in its own folder. It falls
// back to ./shop.db (the previous behavior) when the executable looks
// like a `go run` temp build — its directory containing "go-build" or
// sitting under the OS temp dir — since that path is thrown away after
// the process exits and isn't a sensible place for persistent data.
func defaultDBPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return "./shop.db"
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	dir := filepath.Dir(exePath)

	tmpDir := os.TempDir()
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = resolved
	}

	if strings.Contains(dir, "go-build") || isUnderDir(dir, tmpDir) {
		return "./shop.db"
	}
	return filepath.Join(dir, "shop.db")
}

// isUnderDir reports whether path is dir itself or nested inside it.
func isUnderDir(path, dir string) bool {
	path = filepath.Clean(path)
	dir = filepath.Clean(dir)
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}
