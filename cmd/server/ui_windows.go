//go:build windows

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"shop/internal/client"
	"shop/internal/update"
	"shop/internal/winui"
)

// windowedUI is true where the app shows its own native window instead of opening the user's
// browser.
const windowedUI = true

// windowTitle is also how a second launch finds the first one's window (winui uses it as the
// main window's title).
const windowTitle = "ShopKeeper"

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procFindWindowW      = user32.NewProc("FindWindowW")
	procShowWindow       = user32.NewProc("ShowWindow")
	procSetForeground    = user32.NewProc("SetForegroundWindow")
	procSetProcessDPIAwr = user32.NewProc("SetProcessDPIAware")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procCreateIconFromRE = user32.NewProc("CreateIconFromResourceEx")
	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
)

// appIcon is the program's icon (assets/icon.svg, built by assets/build-icons.sh), several sizes
// in one .ico. The window and taskbar use it; the shortcuts get it from the installer.
//
//go:embed icon_windows.ico
var appIcon []byte

func init() {
	// The window's message loop must run on the thread that created it, and
	// the Go runtime may otherwise move the main goroutine between threads.
	runtime.LockOSThread()
	// Without this Windows scales the whole window as a bitmap on displays set
	// to more than 100%, and the text is blurry.
	_, _, _ = procSetProcessDPIAwr.Call()
}

// setWindowIcon gives the window its title-bar and taskbar icons, picking from the embedded .ico
// the sizes closest to what the display's DPI asks for. Best effort: without an icon the window
// still works.
func setWindowIcon(hwnd uintptr) {
	const (
		wmSetIcon   = 0x0080
		iconSmall   = 0
		iconBig     = 1
		smCxIcon    = 11
		smCxSmIcon  = 49
		iconVersion = 0x00030000
	)
	for _, ic := range []struct {
		which  uintptr
		metric uintptr
	}{{iconSmall, smCxSmIcon}, {iconBig, smCxIcon}} {
		px, _, _ := procGetSystemMetrics.Call(ic.metric)
		data := icoImage(appIcon, int(px))
		if data == nil {
			continue
		}
		h, _, _ := procCreateIconFromRE.Call(
			uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), 1, iconVersion, 0, 0, 0)
		if h != 0 {
			_, _, _ = procSendMessageW.Call(hwnd, wmSetIcon, ic.which, h)
		}
	}
}

// focusExisting brings the window of an already running ShopKeeper to the
// front. False when there is none (the running copy has no window).
func focusExisting() bool {
	title, err := syscall.UTF16PtrFromString(windowTitle)
	if err != nil {
		return false
	}
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
	if hwnd == 0 {
		return false
	}
	const swRestore = 9
	_, _, _ = procShowWindow.Call(hwnd, swRestore)
	_, _, _ = procSetForeground.Call(hwnd)
	return true
}

// reportFatal tells the user why the app could not start. There is no console
// to print to, so it is a message box.
func reportFatal(err error) {
	text, _ := windows.UTF16PtrFromString(fmt.Sprintf("ShopKeeper could not start.\n\n%v", err))
	caption, _ := windows.UTF16PtrFromString(windowTitle)
	_, _ = windows.MessageBox(0, text, caption, windows.MB_OK|windows.MB_ICONERROR)
}

// setupLogging sends the log to a file next to the database: a GUI program
// has no console, so without this every log line would be lost. The previous
// file is kept as shop.log.old once it grows past 2 MB.
func setupLogging(dir string) {
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

// runNativeUI shows the native window on the API handler (no web server, no browser) and blocks
// until the window is closed or quit is closed. It must run on the main goroutine.
func runNativeUI(api http.Handler, port int, quit <-chan struct{}) error {
	return winui.Run(winui.Options{
		Client:  client.New(api, fmt.Sprintf("localhost:%d", port)),
		Updater: updater,
		Version: update.Version,
		Quit:    quit,
		SetIcon: setWindowIcon,
	})
}

// acquireSingleInstance reports whether this is the only ShopKeeper using the database at key.
// The lock lives as long as the process does.
func acquireSingleInstance(key string) bool {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(key)))
	name, err := windows.UTF16PtrFromString(fmt.Sprintf("Local\\ShopKeeper-%x", h.Sum64()))
	if err != nil {
		return true
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if handle != 0 {
			_ = windows.CloseHandle(handle)
		}
		return false
	}
	// Any other failure: carry on rather than refuse to start.
	return true
}
