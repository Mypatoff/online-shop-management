package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackup_OpensAndHasSameRows(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "shop.db")

	src, err := Open(srcPath, "")
	if err != nil {
		t.Fatalf("open source db: %v", err)
	}
	defer src.Close()

	if _, err := src.Exec(
		`INSERT INTO products (name, price, stock, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		"Widget", 999, 10, time.Now().Unix(), time.Now().Unix(),
	); err != nil {
		t.Fatalf("insert product: %v", err)
	}

	destPath := filepath.Join(dir, "shop-2026-10-05.db")
	if err := Backup(src, destPath); err != nil {
		t.Fatalf("Backup: %v", err)
	}

	dst, err := Open(destPath, "")
	if err != nil {
		t.Fatalf("open backup: %v", err)
	}
	defer dst.Close()

	var name string
	if err := dst.QueryRow(`SELECT name FROM products WHERE name = ?`, "Widget").Scan(&name); err != nil {
		t.Fatalf("query backup for product: %v", err)
	}
	if name != "Widget" {
		t.Fatalf("got product name %q, want %q", name, "Widget")
	}
}

func TestRotate_DeletesOnlyOldDatedFiles(t *testing.T) {
	dir := t.TempDir()

	old := DailyBackupName(time.Now().AddDate(0, 0, -40))
	recent := DailyBackupName(time.Now().AddDate(0, 0, -5))
	boundary := DailyBackupName(time.Now().AddDate(0, 0, -30))
	premigration := "shop-premigration-2020-01-01.db"
	other := "notes.txt"

	for _, name := range []string{old, recent, boundary, premigration, other} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := Rotate(dir, 30); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertExists := func(name string, want bool) {
		_, err := os.Stat(filepath.Join(dir, name))
		exists := err == nil
		if exists != want {
			t.Errorf("file %s: exists=%v, want %v", name, exists, want)
		}
	}

	assertExists(old, false)
	assertExists(recent, true)
	assertExists(boundary, true)
	assertExists(premigration, true)
	assertExists(other, true)
}

func TestRotate_HandlesStartFilesAndKeepsToday(t *testing.T) {
	dir := t.TempDir()

	oldDaily := DailyBackupName(time.Now().AddDate(0, 0, -40))
	oldStart := StartBackupName(time.Now().AddDate(0, 0, -40))
	todayDaily := DailyBackupName(time.Now())
	todayStart := StartBackupName(time.Now())
	recentStart := StartBackupName(time.Now().AddDate(0, 0, -5))
	premigration := "shop-premigration-2020-01-01.db"
	other := "notes.txt"

	for _, name := range []string{oldDaily, oldStart, todayDaily, todayStart, recentStart, premigration, other} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	if err := Rotate(dir, 30); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	assertExists := func(name string, want bool) {
		_, err := os.Stat(filepath.Join(dir, name))
		exists := err == nil
		if exists != want {
			t.Errorf("file %s: exists=%v, want %v", name, exists, want)
		}
	}

	assertExists(oldDaily, false)
	assertExists(oldStart, false)
	assertExists(todayDaily, true)
	assertExists(todayStart, true)
	assertExists(recentStart, true)
	assertExists(premigration, true)
	assertExists(other, true)
}

func TestNeedsStartBackup_DoesNotOverwriteExistingToday(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	needed, err := NeedsStartBackup(dir, now)
	if err != nil {
		t.Fatalf("NeedsStartBackup (missing): %v", err)
	}
	if !needed {
		t.Fatal("expected start backup to be needed when today's start file is missing")
	}

	startPath := filepath.Join(dir, StartBackupName(now))
	const marker = "this morning's snapshot"
	if err := os.WriteFile(startPath, []byte(marker), 0o644); err != nil {
		t.Fatalf("seed start file: %v", err)
	}

	needed, err = NeedsStartBackup(dir, now)
	if err != nil {
		t.Fatalf("NeedsStartBackup (present): %v", err)
	}
	if needed {
		t.Fatal("expected no start backup needed once today's start file exists - it must never be overwritten")
	}

	got, err := os.ReadFile(startPath)
	if err != nil {
		t.Fatalf("read start file: %v", err)
	}
	if string(got) != marker {
		t.Fatalf("start file was modified: got %q, want %q", got, marker)
	}
}

func TestNeedsStartupBackup_DoesNotOverwriteExistingToday(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	needed, err := NeedsStartupBackup(dir, now)
	if err != nil {
		t.Fatalf("NeedsStartupBackup (missing): %v", err)
	}
	if !needed {
		t.Fatal("expected backup to be needed when today's file is missing")
	}

	todayPath := filepath.Join(dir, DailyBackupName(now))
	const marker = "already backed up today"
	if err := os.WriteFile(todayPath, []byte(marker), 0o644); err != nil {
		t.Fatalf("seed today's file: %v", err)
	}

	needed, err = NeedsStartupBackup(dir, now)
	if err != nil {
		t.Fatalf("NeedsStartupBackup (present): %v", err)
	}
	if needed {
		t.Fatal("expected no backup needed when today's file already exists")
	}

	got, err := os.ReadFile(todayPath)
	if err != nil {
		t.Fatalf("read today's file: %v", err)
	}
	if string(got) != marker {
		t.Fatalf("today's file was modified: got %q, want %q", got, marker)
	}
}
