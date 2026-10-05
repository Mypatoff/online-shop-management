package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func noBrowser() bool {
	return os.Getenv("NO_BROWSER") == "1"
}

// openBrowser opens the user's default browser at url. It's best
// effort: errors are ignored, since a failure to open a browser
// shouldn't stop the server, and it does nothing if NO_BROWSER=1 is
// set (useful for scripted/headless runs).
func openBrowser(url string) {
	if noBrowser() {
		return
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

// alreadyRunning reports whether a ShopKeeper instance is already
// listening on port by checking that /api/config responds with JSON
// that looks like ours, within a short timeout.
func alreadyRunning(port int) bool {
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/api/config", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return false
	}
	return strings.Contains(string(body), "shop_name")
}
