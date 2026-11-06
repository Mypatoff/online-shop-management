//go:build !windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"time"
)

// windowedUI is false here: the app opens the user's browser. The native
// window is Windows-only.
const windowedUI = false

func showWindow(string, <-chan struct{}) error {
	return errors.New("no native window on this platform")
}

func focusExisting() bool { return false }

func setupLogging(string) {}

// reportFatal tells the user why the app could not start, and keeps a
// double-clicked terminal window open long enough to read it.
func reportFatal(err error) {
	fmt.Fprintln(os.Stderr, "ShopKeeper failed to start:")
	fmt.Fprintln(os.Stderr, err)
	fmt.Fprintln(os.Stderr, "\nPress Enter to close this window (closing automatically in 20 seconds)...")
	waitForEnterOrTimeout(20 * time.Second)
}

// waitForEnterOrTimeout blocks until stdin produces a line (the user
// pressing Enter) or d elapses, whichever comes first - so a
// double-clicked window showing a startup error stays on screen long
// enough to be read instead of vanishing immediately.
func waitForEnterOrTimeout(d time.Duration) {
	enter := make(chan struct{})
	go func() {
		bufio.NewReader(os.Stdin).ReadString('\n') //nolint:errcheck // best effort; timeout covers a closed/absent stdin
		close(enter)
	}()
	select {
	case <-enter:
	case <-time.After(d):
	}
}
