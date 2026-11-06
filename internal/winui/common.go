//go:build windows

// Package winui is ShopKeeper's native Windows interface (Walk). It talks to the shop through
// internal/client, which calls the same handlers the web pages use, so validation and business
// rules are not repeated here.
package winui

import (
	"errors"
	"fmt"
	"time"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"shop/internal/client"
	"shop/internal/money"
)

const appTitle = "ShopKeeper"

// tableModel shows a slice of T in a TableView; cell gives the text of one column.
type tableModel[T any] struct {
	walk.TableModelBase
	rows []T
	cell func(row T, col int) string
}

func (m *tableModel[T]) RowCount() int { return len(m.rows) }

func (m *tableModel[T]) Value(row, col int) interface{} { return m.cell(m.rows[row], col) }

// set replaces the rows. Call it on the UI thread.
func (m *tableModel[T]) set(rows []T) {
	m.rows = rows
	m.PublishRowsReset()
}

// at returns the row at index i (as TableView.CurrentIndex reports it), if there is one.
func (m *tableModel[T]) at(i int) (T, bool) {
	var zero T
	if i < 0 || i >= len(m.rows) {
		return zero, false
	}
	return m.rows[i], true
}

// shop is what every screen shares.
type shop struct {
	mw      *walk.MainWindow
	c       *client.Client
	cfg     client.Config
	dialogs map[*walk.Dialog]struct{} // open dialogs, closed first when the app shuts down

	shuttingDown bool // the app is closing: even a busy dialog must give way
}

// track registers an open dialog; call the result when it closes.
func (s *shop) track(d *walk.Dialog) func() {
	if s.dialogs == nil {
		s.dialogs = map[*walk.Dialog]struct{}{}
	}
	s.dialogs[d] = struct{}{}
	return func() { delete(s.dialogs, d) }
}

// closeAll closes every open dialog, then the main window.
func (s *shop) closeAll() {
	s.shuttingDown = true
	for d := range s.dialogs {
		d.Cancel()
	}
	_ = s.mw.Close()
}

func (s *shop) money(amount int64) string {
	return money.Format(amount, s.cfg.Currency, s.cfg.Decimals)
}

func (s *shop) parseMoney(in string) (int64, error) { return money.Parse(in, s.cfg.Decimals) }

func (s *shop) showError(err error) {
	walk.MsgBox(s.mw, appTitle, err.Error(), walk.MsgBoxIconError)
}

func (s *shop) info(msg string) {
	walk.MsgBox(s.mw, appTitle, msg, walk.MsgBoxIconInformation)
}

func (s *shop) confirm(msg string) bool {
	return walk.MsgBox(s.mw, appTitle, msg, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes
}

// clock formats a unix time as HH:MM in local time.
func clock(unix int64) string { return time.Unix(unix, 0).Local().Format("15:04") }

// fieldError ties a validation message to one form field, so the form can put the cursor there.
type fieldError struct {
	field string
	msg   string
}

func (e *fieldError) Error() string { return e.msg }

func fieldErr(field, msg string) error { return &fieldError{field: field, msg: msg} }

// formField is one row of a form dialog: a text box, or a drop-down when options is set.
type formField struct {
	name    string
	label   string
	value   string       // text, or the value of the selected option
	options []formOption // non-empty: a drop-down
}

type formOption struct{ value, label string }

// formDialog shows a small modal form. onSubmit gets the values by field name; returning an
// error keeps the dialog open and shows the message (a *fieldError also moves the cursor to
// its field). It reports whether the form was submitted successfully.
func (s *shop) formDialog(title, submit string, fields []formField, onSubmit func(values map[string]string) error) bool {
	var dlg *walk.Dialog
	var okBtn, cancelBtn *walk.PushButton
	var errLabel *walk.TextLabel
	edits := make([]*walk.LineEdit, len(fields))
	combos := make([]*walk.ComboBox, len(fields))

	var rows []Widget
	for i, f := range fields {
		rows = append(rows, Label{Text: f.label})
		if len(f.options) > 0 {
			labels := make([]string, len(f.options))
			selected := 0
			for j, o := range f.options {
				labels[j] = o.label
				if o.value == f.value {
					selected = j
				}
			}
			rows = append(rows, ComboBox{AssignTo: &combos[i], Model: labels, CurrentIndex: selected, MinSize: Size{Width: 240}})
		} else {
			rows = append(rows, LineEdit{AssignTo: &edits[i], Text: f.value, MinSize: Size{Width: 240}})
		}
	}

	submitted := false
	err := Dialog{
		AssignTo:      &dlg,
		Title:         title,
		DefaultButton: &okBtn,
		CancelButton:  &cancelBtn,
		MinSize:       Size{Width: 420},
		Layout:        VBox{},
		Children: []Widget{
			Composite{Layout: Grid{Columns: 2, MarginsZero: true}, Children: rows},
			TextLabel{AssignTo: &errLabel, TextColor: walk.RGB(200, 40, 40), MinSize: Size{Width: 380}},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{AssignTo: &okBtn, Text: submit, OnClicked: func() {
						values := make(map[string]string, len(fields))
						for i, f := range fields {
							switch {
							case combos[i] != nil:
								if idx := combos[i].CurrentIndex(); idx >= 0 && idx < len(f.options) {
									values[f.name] = f.options[idx].value
								}
							default:
								values[f.name] = edits[i].Text()
							}
						}
						if err := onSubmit(values); err != nil {
							errLabel.SetText(err.Error())
							var fe *fieldError
							if errors.As(err, &fe) {
								for i, f := range fields {
									if f.name == fe.field && edits[i] != nil {
										_ = edits[i].SetFocus()
									}
								}
							}
							return
						}
						submitted = true
						dlg.Accept()
					}},
					PushButton{AssignTo: &cancelBtn, Text: "Cancel", OnClicked: func() { dlg.Cancel() }},
				},
			},
		},
	}.Create(s.mw)
	if err != nil {
		s.showError(fmt.Errorf("could not open the form: %w", err))
		return false
	}
	defer s.track(dlg)()
	dlg.Run()
	return submitted
}
