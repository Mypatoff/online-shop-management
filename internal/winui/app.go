//go:build windows

package winui

import (
	"fmt"
	"strings"
	"time"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"shop/internal/client"
	"shop/internal/update"
)

// Options is what the window needs from the program.
type Options struct {
	Client  *client.Client
	Updater *update.Updater
	Version string

	// Quit is closed when the program must stop (a signal, or an update that is ready to be
	// installed); the window then closes.
	Quit <-chan struct{}

	// SetIcon gives the window its icon (title bar and taskbar). Optional.
	SetIcon func(hwnd uintptr)
}

// page is a tab that can reload its data when it is shown.
type page interface{ refresh() }

// Run shows the main window and blocks until it is closed. It must be called from the thread
// that owns the UI (main, with the OS thread locked).
func Run(o Options) error {
	app, err := walk.InitApp()
	if err != nil {
		return fmt.Errorf("could not start the window system: %w", err)
	}
	cfg, err := o.Client.Config()
	if err != nil {
		return err
	}
	s := &shop{c: o.Client, cfg: cfg}

	products := newProductsPage(s)
	sell := newSellPage(s)
	pages := []page{products, sell}

	var tabs *walk.TabWidget
	err = MainWindow{
		AssignTo: &s.mw,
		Title:    appTitle, // a second launch finds this window by its title
		Size:     Size{Width: 1100, Height: 720},
		MinSize:  Size{Width: 900, Height: 600},
		Layout:   VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: cfg.ShopName, Font: Font{PointSize: 12, Bold: true}},
					Label{Text: label(o.Version), TextColor: walk.RGB(120, 120, 120)},
					HSpacer{},
					PushButton{Text: "Download backup", OnClicked: s.downloadBackup},
					PushButton{Text: "Update", OnClicked: func() { s.updateDialog(o.Updater) }},
				},
			},
			TabWidget{
				AssignTo: &tabs,
				Pages:    []TabPage{products.tab(), sell.tab()},
			},
		},
	}.Create()
	if err != nil {
		return fmt.Errorf("could not create the window: %w", err)
	}
	if o.SetIcon != nil {
		o.SetIcon(uintptr(s.mw.Handle()))
	}

	tabs.CurrentIndexChanged().Attach(func() {
		if i := tabs.CurrentIndex(); i >= 0 && i < len(pages) {
			pages[i].refresh()
		}
	})
	for _, p := range pages {
		p.refresh()
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-o.Quit:
			walk.App().Synchronize(s.closeAll)
		case <-done:
		}
	}()

	app.Run()
	return nil
}

// downloadBackup saves a snapshot of the database wherever the user chooses.
func (s *shop) downloadBackup() {
	dlg := walk.FileDialog{
		Title:    "Save backup",
		FilePath: fmt.Sprintf("shop-backup-%s.db", time.Now().Format("2006-01-02")),
		Filter:   "Database (*.db)|*.db|All files (*.*)|*.*",
	}
	ok, err := dlg.ShowSave(s.mw)
	if err != nil {
		s.showError(err)
		return
	}
	if !ok {
		return
	}
	path := dlg.FilePath
	if !strings.Contains(lastElem(path), ".") {
		path += ".db"
	}
	if err := s.c.BackupTo(path); err != nil {
		s.showError(fmt.Errorf("could not save the backup: %w", err))
		return
	}
	s.info("Backup saved:\r\n" + path)
}

func lastElem(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}
