//go:build windows

package winui

import (
	"context"
	"fmt"
	"time"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"shop/internal/update"
)

func label(version string) string {
	if version != "" && version[0] >= '0' && version[0] <= '9' {
		return "v" + version
	}
	return version
}

// updateDialog is the "Update" window: it checks GitHub, shows what is new, and installs. The
// download runs in the background; when it is verified the updater signals the app (see
// cmd/server) to shut down, install and restart.
func (s *shop) updateDialog(u *update.Updater) {
	var dlg *walk.Dialog
	var info *walk.TextLabel
	var notes *walk.TextEdit
	var bar *walk.ProgressBar
	var goBtn, closeBtn *walk.PushButton

	closed, busy := false, false
	action := func() {}
	// ui runs f on the UI thread, unless the dialog has gone.
	ui := func(f func()) {
		walk.App().Synchronize(func() {
			if !closed {
				f()
			}
		})
	}

	setIdle := func(text string, withGo bool, goText string, do func()) {
		info.SetText(text)
		bar.SetVisible(false)
		goBtn.SetVisible(withGo)
		goBtn.SetEnabled(true)
		goBtn.SetText(goText)
		closeBtn.SetEnabled(true)
		action = do
		busy = false
	}

	var check func()
	var install func()

	check = func() {
		info.SetText("Checking GitHub for a newer version…")
		notes.SetVisible(false)
		goBtn.SetVisible(false)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			res, err := u.Check(ctx)
			ui(func() {
				switch {
				case err != nil:
					setIdle(err.Error(), true, "Try again", check)
				case !res.Available:
					msg := res.Reason
					if msg == "" {
						msg = "You have the latest version."
					}
					setIdle(fmt.Sprintf("%s\r\nInstalled: %s", msg, label(res.Current))+latest(res.Latest), false, "", nil)
				case !res.CanApply:
					setIdle(fmt.Sprintf("Version %s is available (you have %s).\r\n%s\r\nRelease page: %s",
						label(res.Latest), label(res.Current), res.Reason, res.URL), false, "", nil)
				default:
					if res.Notes != "" {
						notes.SetText(toCRLF(res.Notes))
						notes.SetVisible(true)
					}
					setIdle(fmt.Sprintf("Version %s is available (you have %s).\r\nYour products, sales and backups are kept. ShopKeeper takes a backup, then restarts by itself.",
						label(res.Latest), label(res.Current)), true, "Update now", install)
				}
			})
		}()
	}

	install = func() {
		busy = true
		goBtn.SetEnabled(false)
		closeBtn.SetEnabled(false)
		notes.SetVisible(false)
		bar.SetValue(0)
		bar.SetVisible(true)
		info.SetText("Starting the update…")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			err := u.Start(ctx)
			cancel()
			if err != nil {
				ui(func() {
					setIdle("The update did not finish. Nothing was changed.\r\n"+err.Error(), true, "Try again", check)
				})
				return
			}
			for {
				time.Sleep(400 * time.Millisecond)
				st := u.Status()
				switch st.State {
				case "error":
					ui(func() {
						setIdle("The update did not finish. Nothing was changed.\r\n"+st.Error, true, "Try again", check)
					})
					return
				case "restarting":
					ui(func() {
						bar.SetValue(100)
						info.SetText("Installing and restarting…")
					})
					return // the app now shuts down and starts the new version
				default:
					ui(func() {
						bar.SetValue(st.Percent)
						info.SetText(fmt.Sprintf("Downloading the new version… %d%%", st.Percent))
					})
				}
			}
		}()
	}

	err := Dialog{
		AssignTo:     &dlg,
		Title:        "Update ShopKeeper",
		CancelButton: &closeBtn,
		MinSize:      Size{Width: 480},
		Layout:       VBox{},
		Children: []Widget{
			TextLabel{AssignTo: &info, MinSize: Size{Width: 440, Height: 50}},
			TextEdit{AssignTo: &notes, ReadOnly: true, VScroll: true, MinSize: Size{Height: 120}, Visible: false},
			ProgressBar{AssignTo: &bar, MinValue: 0, MaxValue: 100, Visible: false},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &goBtn, Text: "Update now", Visible: false, OnClicked: func() { action() }},
					PushButton{AssignTo: &closeBtn, Text: "Close", OnClicked: func() {
						if !busy {
							dlg.Cancel()
						}
					}},
				},
			},
		},
	}.Create(s.mw)
	if err != nil {
		s.showError(fmt.Errorf("could not open the update window: %w", err))
		return
	}
	dlg.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if busy && !s.shuttingDown {
			*canceled = true // not halfway through an update
			return
		}
		closed = true
	})
	defer s.track(dlg)()
	check()
	dlg.Run()
	closed = true
}

func latest(v string) string {
	if v == "" {
		return ""
	}
	return " · Latest: " + label(v)
}

// toCRLF makes release notes show properly line by line in an edit box.
func toCRLF(s string) string {
	out := make([]byte, 0, len(s)+16)
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\r':
			// dropped; the \n that follows (or this one) becomes CR LF
			if i+1 >= len(s) || s[i+1] != '\n' {
				out = append(out, '\r', '\n')
			}
		case s[i] == '\n':
			out = append(out, '\r', '\n')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
