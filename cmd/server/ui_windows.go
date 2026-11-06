//go:build windows

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

// windowedUI is true where the app shows its own window instead of
// opening the user's browser.
const windowedUI = true

// windowTitle is also how a second launch finds the first one's window.
const windowTitle = "ShopKeeper"

// webView2Download is Microsoft's page for the runtime the window needs.
const webView2Download = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"

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

// showWindow opens the app in a native window on url and blocks until the
// window is closed, or quit is closed (the server died, or the process was
// told to stop). It must be called from the main goroutine.
func showWindow(url string, quit <-chan struct{}) error {
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  webViewDataDir(),
		WindowOptions: webview2.WindowOptions{
			Title:  windowTitle,
			Width:  1280,
			Height: 800,
			Center: true,
		},
	})
	if w == nil {
		return errors.New("the window could not be created: Microsoft Edge WebView2 Runtime is not installed.\n\n" +
			"Install it (free, about 2 MB, needs internet once) from:\n" + webView2Download)
	}
	defer w.Destroy()
	setWindowIcon(uintptr(w.Window()))
	w.SetSize(900, 600, webview2.HintMin)
	w.Navigate(url)

	go func() {
		<-quit
		// Not Terminate(): that posts the quit message to the calling
		// thread, which is not the one running the loop. Destroy posts
		// WM_CLOSE to the window, and the loop ends from there.
		w.Destroy()
	}()
	w.Run()
	return nil
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

// webViewDataDir is where WebView2 keeps its cache. Under the user's profile
// rather than next to the exe, which may sit in a folder only an
// administrator can write to.
func webViewDataDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return "" // WebView2 picks its own default
	}
	return filepath.Join(base, "ShopKeeper", "webview")
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
