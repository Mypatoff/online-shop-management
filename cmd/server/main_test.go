package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// portOf returns the port a test server listens on.
func portOf(t *testing.T, srv *httptest.Server) int {
	t.Helper()
	_, p, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// A second Listen on a taken port must be recognised as "address in use" with
// the error the OS really returns. On Windows that is the Winsock error
// WSAEADDRINUSE rather than Go's syscall.EADDRINUSE, which is the bug this
// guards against: CI runs this test on Windows too.
func TestIsAddrInUseRecognisesTheRealListenError(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()

	_, err = net.Listen("tcp", first.Addr().String())
	if err == nil {
		t.Fatal("second Listen on the same address succeeded")
	}
	if !isAddrInUse(err) {
		t.Fatalf("isAddrInUse(%v) = false for the error the OS returned", err)
	}
	if isAddrInUse(net.ErrClosed) {
		t.Error("isAddrInUse accepted an unrelated error")
	}
}

func TestAlreadyRunningRecognisesShopKeeper(t *testing.T) {
	ours := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"shop_name":"Corner Shop","currency":"so'm","decimals":0}`))
	}))
	defer ours.Close()
	if !alreadyRunning(portOf(t, ours)) {
		t.Error("a running ShopKeeper was not recognised")
	}

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>some other program</html>"))
	}))
	defer other.Close()
	if alreadyRunning(portOf(t, other)) {
		t.Error("another program on the port was taken for ShopKeeper")
	}

	// Nothing listening at all.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if alreadyRunning(port) {
		t.Error("an empty port was taken for ShopKeeper")
	}
}
