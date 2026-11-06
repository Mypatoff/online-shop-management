package main

import (
	"log"
	"os"
	"path/filepath"
)

// logToFile sends the log to shop.log in dir. A program with no console (the
// Windows window, a Linux launcher) would otherwise lose every log line. The
// previous file is kept as shop.log.old once it grows past 2 MB.
func logToFile(dir string) {
	path := filepath.Join(dir, "shop.log")
	if info, err := os.Stat(path); err == nil && info.Size() > 2<<20 {
		_ = os.Remove(path + ".old")
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	log.SetOutput(f)
}
