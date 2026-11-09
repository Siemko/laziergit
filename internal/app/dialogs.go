package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/core"
	"github.com/jesseduffield/lazygit/pkg/commands/oscommands"
)

type dialogKind int

const (
	dialogConfirm dialogKind = iota
	dialogPrompt
	dialogCredential
)

// dialog is the one dialog showing.
type dialog struct {
	kind      dialogKind
	title     string
	message   string
	button    string
	multiline bool
	value     string
	secret    bool
	onOK      func(value string)
	onCancel  func()
	open      bool
}

// asking reports whether a credential prompt shows, which git waits on:
// another dialog must not take its place.
func (a *App) asking() bool { return a.dialog != nil && a.dialog.kind == dialogCredential }

func (a *App) confirm(title, message, button string, ok func()) {
	if a.asking() {
		return
	}
	a.dialog = &dialog{kind: dialogConfirm, title: title, message: message, button: button, open: true,
		onOK: func(string) { ok() }}
}

func (a *App) prompt(title, value, button string, multiline bool, ok func(string)) {
	if a.asking() {
		return
	}
	a.dialog = &dialog{kind: dialogPrompt, title: title, value: value, button: button, multiline: multiline, open: true, onOK: ok}
}

func (a *App) askCredential(kind core.Credential, ch chan string) {
	titles := map[core.Credential]string{
		oscommands.Username:   "Username",
		oscommands.Password:   "Password",
		oscommands.Passphrase: "SSH key passphrase",
		oscommands.PIN:        "PIN",
		oscommands.Token:      "Token",
	}
	// git reads a line: the answer and Enter, as lazygit's own prompt
	// sends it. Cancelling sends an empty line, which fails the command
	// rather than leave it waiting with the repository taken.
	answered := false
	id := len(a.credentials)
	answer := func(v string) {
		if !answered {
			answered = true
			ch <- v + "\n"
			delete(a.credentials, id)
		}
	}
	if a.closed {
		answer("")
		return
	}
	if a.credentials == nil {
		a.credentials = map[int]func(string){}
	}
	for a.credentials[id] != nil {
		id++
	}
	a.credentials[id] = answer
	a.dialog = &dialog{
		kind: dialogCredential, title: titles[kind], message: "git is asking for your " + titles[kind] + ".",
		button: "Continue", secret: kind != oscommands.Username, open: true,
		onOK:     answer,
		onCancel: func() { answer("") },
	}
}

func (a *App) dialogs(c *ui.Context, pal *palette) {
	d := a.dialog
	if d == nil {
		return
	}
	finish := func(ok bool) {
		a.dialog = nil
		if ok && d.onOK != nil {
			d.onOK(d.value)
		} else if !ok && d.onCancel != nil {
			d.onCancel()
		}
	}
	if d.kind == dialogConfirm {
		switch ui.AlertDialog(c, &d.open, d.title, d.message, "Cancel", d.button) {
		case 1:
			finish(true)
		case 0:
			finish(false)
		default:
			if !d.open {
				finish(false)
			}
		}
		return
	}
	ui.Modal(c, &d.open, func() {
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Text(c, d.title).FontSize(15).Bold()
			if d.message != "" {
				ui.Text(c, d.message).FontSize(12).TextColor(pal.muted)
			}
			submitted := false
			switch {
			case d.multiline:
				ui.TextArea(c.Key("value"), &d.value).Height(110).AutoFocus().Font(codeFont).FontSize(12)
				submitted = c.Shortcut(ui.Cmd, ui.KeyEnter)
			case d.secret:
				submitted = ui.TextInput(c.Key("value"), &d.value).Password().AutoFocus().Submitted()
			default:
				submitted = ui.TextInput(c.Key("value"), &d.value).AutoFocus().Submitted()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					finish(false)
				}
				if ui.PrimaryButton(c, d.button).Clicked() || submitted {
					finish(true)
				}
			})
		})
	})
	if a.dialog == d && !d.open {
		finish(false)
	}
}
