//go:build !windows

package main

import (
	"errors"
	"syscall"
)

// isAddrInUse reports whether err from net.Listen means the port is taken.
func isAddrInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }
