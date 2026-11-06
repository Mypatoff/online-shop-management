package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestParseVersionAndCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int // sign of compare(a, b); 2 = a is not a version
	}{
		{"v1.2.0", "1.1.9", 1},
		{"1.0.0", "v1.0.0", 0},
		{"1.0", "1.0.1", -1},
		{"v2.0.0-rc1", "1.9.9", 1},
		{"dev", "1.0.0", 2},
		{"", "1.0.0", 2},
		{"1.2.3.4", "1.0.0", 2},
		{"1.x.0", "1.0.0", 2},
	}
	for _, tt := range tests {
		a, ok := parseVersion(tt.a)
		if tt.want == 2 {
			if ok {
				t.Errorf("parseVersion(%q) ok, want not a version", tt.a)
			}
			continue
		}
		b, _ := parseVersion(tt.b)
		if !ok {
			t.Errorf("parseVersion(%q) not ok", tt.a)
			continue
		}
		if got := compare(a, b); got != tt.want {
			t.Errorf("compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

// fakeGitHub serves one release whose files are the given contents, and returns an Updater
// aimed at it with a temp "installed" executable.
type fakeGitHub struct {
	srv       *httptest.Server
	tag       string
	exe       []byte
	sums      string // "" = use the real checksum; "none" = no checksum asset
	exePath   string
	updater   *Updater
	noRelease bool
}

func newFake(t *testing.T, current, tag string, exe []byte) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{tag: tag, exe: exe}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if f.noRelease {
			http.NotFound(w, r)
			return
		}
		assets := fmt.Sprintf(`{"name":"shopkeeper-linux","browser_download_url":"%s/dl/shopkeeper-linux"}`, f.srv.URL)
		if f.sums != "none" {
			assets += fmt.Sprintf(`,{"name":"SHA256SUMS.txt","browser_download_url":"%s/dl/SHA256SUMS.txt"}`, f.srv.URL)
		}
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example/r","body":"notes","assets":[%s]}`, f.tag, assets)
	})
	mux.HandleFunc("/dl/shopkeeper-linux", func(w http.ResponseWriter, r *http.Request) { w.Write(f.exe) })
	mux.HandleFunc("/dl/SHA256SUMS.txt", func(w http.ResponseWriter, r *http.Request) {
		sum := f.sums
		if sum == "" {
			h := sha256.Sum256(f.exe)
			sum = hex.EncodeToString(h[:])
		}
		fmt.Fprintf(w, "%s  shopkeeper-linux\n", sum)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)

	dir := t.TempDir()
	f.exePath = filepath.Join(dir, "shopkeeper")
	if err := os.WriteFile(f.exePath, []byte("\x7fELFold"), 0o755); err != nil {
		t.Fatal(err)
	}
	u := New(current)
	u.Repo = "o/r"
	u.APIBase = f.srv.URL
	u.DownloadPrefix = f.srv.URL + "/dl/"
	u.Client = f.srv.Client()
	u.GOOS, u.GOARCH = "linux", "amd64"
	u.ExePath = f.exePath
	f.updater = u
	return f
}

func TestCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("newer release is available", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", []byte("\x7fELFnew"))
		info, err := f.updater.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Available || !info.CanApply || info.Latest != "1.1.0" || info.Notes != "notes" {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("same version", func(t *testing.T) {
		f := newFake(t, "1.1.0", "v1.1.0", nil)
		info, err := f.updater.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Available || info.CanApply {
			t.Fatalf("info = %+v, want up to date", info)
		}
	})
	t.Run("dev build never applies", func(t *testing.T) {
		f := newFake(t, "dev", "v9.0.0", nil)
		info, err := f.updater.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Available || info.CanApply || info.Latest != "9.0.0" {
			t.Fatalf("info = %+v", info)
		}
	})
	t.Run("no checksum file cannot apply", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", nil)
		f.sums = "none"
		info, err := f.updater.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Available || info.CanApply {
			t.Fatalf("info = %+v, want available but not applicable", info)
		}
	})
	t.Run("no release published", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", nil)
		f.noRelease = true
		if _, err := f.updater.Check(ctx); err == nil {
			t.Fatal("want an error")
		}
	})
	t.Run("unsupported platform", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", nil)
		f.updater.GOOS = "darwin"
		info, _ := f.updater.Check(ctx)
		if !info.Available || info.CanApply {
			t.Fatalf("info = %+v", info)
		}
	})
}

func waitFor(t *testing.T, u *Updater) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := u.Status(); st.State != "downloading" {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	return u.Status()
}

func TestStartInstall(t *testing.T) {
	newExe := []byte("\x7fELFnew-version")
	f := newFake(t, "1.0.0", "v1.1.0", newExe)
	u := f.updater

	if err := u.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := waitFor(t, u); st.State != "restarting" {
		t.Fatalf("status = %+v", st)
	}
	select {
	case <-u.Restart():
	case <-time.After(time.Second):
		t.Fatal("Restart was not signalled after the download")
	}
	if !u.Pending() {
		t.Fatal("Pending = false after download")
	}
	// Until Install, the running program is untouched.
	if got, _ := os.ReadFile(f.exePath); string(got) != "\x7fELFold" {
		t.Fatalf("exe changed before Install: %q", got)
	}

	if err := u.Install(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(f.exePath); string(got) != string(newExe) {
		t.Fatalf("exe = %q, want the new version", got)
	}
	if got, _ := os.ReadFile(f.exePath + ".old"); string(got) != "\x7fELFold" {
		t.Fatalf(".old = %q, want the previous version", got)
	}
	// Windows has no executable bit.
	if info, err := os.Stat(f.exePath); err != nil || (runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0) {
		t.Fatalf("new exe is not executable: %v %v", info, err)
	}
	if u.Pending() {
		t.Fatal("still pending after Install")
	}

	u.CleanupLeftovers()
	if _, err := os.Stat(f.exePath + ".old"); !os.IsNotExist(err) {
		t.Fatalf(".old not cleaned up: %v", err)
	}
}

func TestStartRejectsBadDownloads(t *testing.T) {
	t.Run("checksum mismatch", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", []byte("\x7fELFnew"))
		f.sums = fmt.Sprintf("%064d", 7)
		if err := f.updater.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if st := waitFor(t, f.updater); st.State != "error" {
			t.Fatalf("status = %+v, want error", st)
		}
		assertUntouched(t, f)
	})
	t.Run("not a program", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", []byte("<html>not found</html>"))
		if err := f.updater.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if st := waitFor(t, f.updater); st.State != "error" {
			t.Fatalf("status = %+v, want error", st)
		}
		assertUntouched(t, f)
	})
	t.Run("unexpected download address", func(t *testing.T) {
		f := newFake(t, "1.0.0", "v1.1.0", []byte("\x7fELFnew"))
		f.updater.DownloadPrefix = "https://github.com/o/r/releases/download/"
		if err := f.updater.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		if st := waitFor(t, f.updater); st.State != "error" {
			t.Fatalf("status = %+v, want error", st)
		}
		assertUntouched(t, f)
	})
	t.Run("nothing to apply", func(t *testing.T) {
		f := newFake(t, "1.1.0", "v1.1.0", nil)
		if err := f.updater.Start(context.Background()); err == nil {
			t.Fatal("want an error when already up to date")
		}
	})
}

func assertUntouched(t *testing.T, f *fakeGitHub) {
	t.Helper()
	if got, _ := os.ReadFile(f.exePath); string(got) != "\x7fELFold" {
		t.Fatalf("exe changed: %q", got)
	}
	if _, err := os.Stat(f.exePath + ".new"); !os.IsNotExist(err) {
		t.Fatalf("staged file left behind: %v", err)
	}
	if f.updater.Pending() {
		t.Fatal("Pending after a failed download")
	}
}

func TestFindChecksum(t *testing.T) {
	h := fmt.Sprintf("%064x", 255)
	sums := h + "  ShopKeeper.exe\n" + h[:63] + "0 *shopkeeper-linux\nbroken line\n"
	if got, ok := findChecksum(sums, "ShopKeeper.exe"); !ok || got != h {
		t.Errorf("got %q %v", got, ok)
	}
	if _, ok := findChecksum(sums, "shopkeeper-linux"); !ok {
		t.Error("binary-mode (*name) line not found")
	}
	if _, ok := findChecksum(sums, "missing"); ok {
		t.Error("found a name that is not listed")
	}
}
