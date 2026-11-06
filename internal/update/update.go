// Package update checks GitHub for a newer ShopKeeper release and replaces the running
// program with it.
//
// The flow is split in two so a failed update can never leave the shop without a working app:
//
//  1. Start (while the app keeps running) downloads the release asset next to the executable
//     as "<exe>.new" and verifies it against the release's SHA256SUMS.txt.
//  2. Once the server has shut down cleanly (last backup taken, database closed), main calls
//     Install, which swaps the files, and Relaunch, which starts the new program.
//
// Nothing here touches shop.db, backups/ or shop.log.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Version is the running program's version, set at build time:
//
//	go build -ldflags "-X shop/internal/update.Version=1.2.3"
//
// A plain "go run" or "go build" leaves it as "dev", which never offers or applies updates.
var Version = "dev"

// DefaultRepo is the GitHub repository releases are read from.
const DefaultRepo = "Mypatoff/online-shop-management"

// Where releases are read from. They are variables only so a test build can point them at a
// local fake with -ldflags "-X shop/internal/update.apiBase=http://127.0.0.1:9911"; there is
// deliberately no run-time setting for them.
var (
	apiBase        = "https://api.github.com"
	downloadPrefix = "https://github.com/" + DefaultRepo + "/releases/download/"
)

const (
	checksumsAsset = "SHA256SUMS.txt"
	maxAPIBody     = 1 << 20   // 1 MiB of release JSON
	maxChecksums   = 1 << 16   // 64 KiB
	maxDownload    = 200 << 20 // 200 MiB; the exe is about 10
)

// Info is the answer to "is there a newer version?".
type Info struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	CanApply    bool   `json:"can_apply"`
	Reason      string `json:"reason,omitempty"` // why it cannot be applied, or why there is nothing to apply
	URL         string `json:"url,omitempty"`    // the release page
	Notes       string `json:"notes,omitempty"`
	PublishedAt string `json:"published_at,omitempty"`
}

// Status describes a running or finished update, for the progress display.
type Status struct {
	State   string `json:"state"` // idle, downloading, restarting, error
	Percent int    `json:"percent"`
	Error   string `json:"error,omitempty"`
}

// Updater checks for and applies updates. Use New; the exported fields are for tests.
type Updater struct {
	Current        string
	Repo           string
	APIBase        string // GitHub's API
	DownloadPrefix string // asset URLs must start with this (GitHub's release download URL)
	Client         *http.Client
	GOOS, GOARCH   string
	ExePath        string

	restart chan struct{}

	mu     sync.Mutex
	status Status
	staged string
}

// New returns an Updater for the running program.
func New(current string) *Updater {
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
	}
	return &Updater{
		Current:        current,
		Repo:           DefaultRepo,
		APIBase:        apiBase,
		DownloadPrefix: downloadPrefix,
		Client:         &http.Client{Timeout: 10 * time.Minute},
		GOOS:           runtime.GOOS,
		GOARCH:         runtime.GOARCH,
		ExePath:        exe,
		restart:        make(chan struct{}, 1),
		status:         Status{State: "idle"},
	}
}

// Restart is signalled once the new version is downloaded and verified and the app should
// shut down so Install and Relaunch can run.
func (u *Updater) Restart() <-chan struct{} { return u.restart }

// assetName is the release file for this platform, or "" if releases do not have one.
func (u *Updater) assetName() string {
	switch {
	case u.GOOS == "windows" && u.GOARCH == "amd64":
		return "ShopKeeper.exe"
	case u.GOOS == "linux" && u.GOARCH == "amd64":
		return "shopkeeper-linux"
	}
	return ""
}

type release struct {
	TagName     string  `json:"tag_name"`
	HTMLURL     string  `json:"html_url"`
	Body        string  `json:"body"`
	PublishedAt string  `json:"published_at"`
	Assets      []asset `json:"assets"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}

func (r release) asset(name string) (asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return asset{}, false
}

func (u *Updater) get(ctx context.Context, url, accept string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "ShopKeeper/"+u.Current)
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub (is the internet connected?): %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errors.New("no release has been published yet")
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("GitHub is limiting requests right now; try again in a few minutes")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("response from GitHub is too large")
	}
	return body, nil
}

func (u *Updater) latest(ctx context.Context) (release, error) {
	body, err := u.get(ctx, u.APIBase+"/repos/"+u.Repo+"/releases/latest", "application/vnd.github+json", maxAPIBody)
	if err != nil {
		return release{}, err
	}
	var r release
	if err := json.Unmarshal(body, &r); err != nil || r.TagName == "" {
		return release{}, errors.New("unexpected answer from GitHub")
	}
	return r, nil
}

// Check asks GitHub for the latest release and compares it with the running version.
func (u *Updater) Check(ctx context.Context) (Info, error) {
	info, _, err := u.check(ctx)
	return info, err
}

func (u *Updater) check(ctx context.Context) (Info, release, error) {
	rel, err := u.latest(ctx)
	if err != nil {
		return Info{Current: u.Current}, release{}, err
	}
	info := Info{
		Current:     u.Current,
		Latest:      strings.TrimPrefix(rel.TagName, "v"),
		URL:         rel.HTMLURL,
		Notes:       rel.Body,
		PublishedAt: rel.PublishedAt,
	}

	cur, curOK := parseVersion(u.Current)
	lat, latOK := parseVersion(rel.TagName)
	switch {
	case !curOK:
		info.Reason = "This is a development build, so it cannot update itself."
		return info, rel, nil
	case !latOK:
		info.Reason = "The latest release has an unrecognised version number."
		return info, rel, nil
	}
	info.Available = compare(lat, cur) > 0
	if !info.Available {
		info.Reason = "You have the latest version."
		return info, rel, nil
	}

	name := u.assetName()
	if name == "" {
		info.Reason = "Automatic update is not available for " + u.GOOS + "/" + u.GOARCH + "; download it from the release page."
		return info, rel, nil
	}
	if _, ok := rel.asset(name); !ok {
		info.Reason = "The release has no " + name + " file."
		return info, rel, nil
	}
	if _, ok := rel.asset(checksumsAsset); !ok {
		info.Reason = "The release has no " + checksumsAsset + " file, so the download cannot be verified."
		return info, rel, nil
	}
	info.CanApply = true
	return info, rel, nil
}

// Status returns the progress of the update started with Start.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *Updater) setStatus(s Status) {
	u.mu.Lock()
	u.status = s
	u.mu.Unlock()
}

// Start begins downloading the latest version in the background. Progress is reported by
// Status; when the file is verified, Restart is signalled. It returns an error right away
// if an update is already running or there is nothing to apply.
func (u *Updater) Start(ctx context.Context) error {
	info, rel, err := u.check(ctx)
	if err != nil {
		return err
	}
	if !info.CanApply {
		return errors.New(info.Reason)
	}

	u.mu.Lock()
	if u.status.State == "downloading" || u.status.State == "restarting" {
		u.mu.Unlock()
		return errors.New("an update is already in progress")
	}
	u.status = Status{State: "downloading"}
	u.mu.Unlock()

	go func() {
		dlCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		path, err := u.download(dlCtx, rel)
		if err != nil {
			u.setStatus(Status{State: "error", Error: err.Error()})
			return
		}
		u.mu.Lock()
		u.staged = path
		u.status = Status{State: "restarting", Percent: 100}
		u.mu.Unlock()
		select {
		case u.restart <- struct{}{}:
		default:
		}
	}()
	return nil
}

func (u *Updater) download(ctx context.Context, rel release) (string, error) {
	exeAsset, _ := rel.asset(u.assetName())
	sumsAsset, _ := rel.asset(checksumsAsset)
	for _, a := range []asset{exeAsset, sumsAsset} {
		if !strings.HasPrefix(a.URL, u.DownloadPrefix) {
			return "", fmt.Errorf("refusing to download %s from an unexpected address", a.Name)
		}
	}

	sums, err := u.get(ctx, sumsAsset.URL, "text/plain", maxChecksums)
	if err != nil {
		return "", fmt.Errorf("could not get %s: %w", checksumsAsset, err)
	}
	want, ok := findChecksum(string(sums), exeAsset.Name)
	if !ok {
		return "", fmt.Errorf("%s does not list %s", checksumsAsset, exeAsset.Name)
	}

	staged := u.ExePath + ".new"
	got, err := u.downloadFile(ctx, exeAsset.URL, staged)
	if err != nil {
		_ = os.Remove(staged)
		return "", err
	}
	if got != want {
		_ = os.Remove(staged)
		return "", errors.New("the downloaded file is damaged (checksum mismatch); nothing was changed")
	}
	if err := checkMagic(staged, u.GOOS); err != nil {
		_ = os.Remove(staged)
		return "", err
	}
	return staged, nil
}

// downloadFile saves url to path and returns the file's SHA-256 (hex).
func (u *Updater) downloadFile(ctx context.Context, url, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "ShopKeeper/"+u.Current)
	resp, err := u.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed (is the internet connected?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed: GitHub answered %s", resp.Status)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return "", fmt.Errorf("cannot write next to the program (%s): %w. Reinstall with the installer, or run it as administrator", filepath.Dir(path), err)
	}
	defer f.Close()

	h := sha256.New()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 64<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if done += int64(n); done > maxDownload {
				return "", errors.New("download is larger than expected")
			}
			h.Write(buf[:n])
			if _, err := f.Write(buf[:n]); err != nil {
				return "", fmt.Errorf("could not save the download: %w", err)
			}
			if total > 0 {
				u.setStatus(Status{State: "downloading", Percent: int(done * 99 / total)})
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", fmt.Errorf("download interrupted: %w", rerr)
		}
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("could not save the download: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Pending reports whether a verified update is waiting for Install.
func (u *Updater) Pending() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.staged != ""
}

// Install replaces the executable with the downloaded one. Call it only after the app has
// stopped using the database. If it fails the old executable is left in place. The old file
// is kept as "<exe>.old" until the next start (Windows cannot delete a running program).
func (u *Updater) Install() error {
	u.mu.Lock()
	staged := u.staged
	u.mu.Unlock()
	if staged == "" {
		return nil
	}
	old := u.ExePath + ".old"
	_ = os.Remove(old)
	if err := os.Rename(u.ExePath, old); err != nil {
		_ = os.Remove(staged)
		return fmt.Errorf("could not replace the program: %w", err)
	}
	if err := os.Rename(staged, u.ExePath); err != nil {
		if rerr := os.Rename(old, u.ExePath); rerr != nil {
			return fmt.Errorf("could not install the update (%v) and could not put the old program back (%v): the old one is %s", err, rerr, old)
		}
		_ = os.Remove(staged)
		return fmt.Errorf("could not install the update: %w", err)
	}
	u.mu.Lock()
	u.staged = ""
	u.mu.Unlock()
	return nil
}

// Relaunch starts the program at ExePath as a separate process. Call it after the server
// has released its port and database.
func (u *Updater) Relaunch() error {
	cmd := exec.Command(u.ExePath)
	cmd.Dir = filepath.Dir(u.ExePath)
	return cmd.Start()
}

// CleanupLeftovers removes files an earlier update left beside the executable. Call it
// at start-up.
func (u *Updater) CleanupLeftovers() {
	_ = os.Remove(u.ExePath + ".old")
	_ = os.Remove(u.ExePath + ".new")
}

func checkMagic(path, goos string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return errors.New("the downloaded file is empty")
	}
	want := "\x7fELF"
	if goos == "windows" {
		want = "MZ"
	}
	if !strings.HasPrefix(string(head), want) {
		return errors.New("the downloaded file is not a program for this system")
	}
	return nil
}

// findChecksum reads "sha256  name" lines (sha256sum / Get-FileHash style) and returns the
// lower-case hash for name.
func findChecksum(sums, name string) (string, bool) {
	for _, line := range strings.Split(sums, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == name && len(fields[0]) == 64 {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// parseVersion reads "v1.2.3", "1.2" or "1.2.3-rc1" as numbers; anything else ("dev") is
// not a version.
func parseVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if s == "" || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func compare(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] > b[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}
