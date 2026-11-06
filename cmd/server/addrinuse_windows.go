//go:build windows

package main

import (
	"errors"
	"syscall"

	"golang.org/x/sys/windows"
)

// isAddrInUse reports whether err from net.Listen means the port is taken.
// Windows reports the Winsock error WSAEADDRINUSE (10048), which is not the
// same value as Go's syscall.EADDRINUSE, so both are checked.
func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windows.WSAEADDRINUSE)
}
