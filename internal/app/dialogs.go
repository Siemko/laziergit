package app

import (
	"strings"

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

// prompt asks for a value, and gives the dialog that asks: nil when a
// credential prompt shows, which it doesn't replace.
func (a *App) prompt(title, value, button string, multiline bool, ok func(string)) *dialog {
	if a.asking() {
		return nil
	}
	a.dialog = &dialog{kind: dialogPrompt, title: title, value: value, button: button, multiline: multiline, open: true, onOK: ok}
	return a.dialog
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
		kind: dialogCredential, title: titles[kind], message: "git is asking for your " + strings.ToLower(titles[kind]) + ".",
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
	tt := c.Theme()
	panelBg := pal.raised
	sheet := func(width float32, body func()) {
		ui.DialogBase(c, &d.open, func(back, panel ui.Element) {
			scrim := ui.RGBA(0, 0, 0, 0.28)
			if tt.Dark {
				scrim = ui.RGBA(0, 0, 0, 0.5)
			}
			back.Background(scrim)
			panel.Width(width).Padding(20).Gap(12).Radius(12).Background(panelBg).Border(1, pal.border).
				Shadow(0, 16, 40, 0, ui.RGBA(0, 0, 0, 0.22))
			body()
		})
	}
	if d.kind == dialogConfirm {
		danger := false
		for _, verb := range []string{"Discard", "Delete", "Drop", "Abort", "Force"} {
			if strings.HasPrefix(d.button, verb) {
				danger = true
			}
		}
		sheet(360, func() {
			ui.Column(c).Gap(6).Children(func() {
				ui.Text(c, d.title).FontSize(14).FontWeight(600)
				if d.message != "" {
					ui.Text(c, d.message).FontSize(12).TextColor(pal.muted)
				}
			})
			ui.Row(c).Gap(8).Justify(ui.End).Margin(6, 0, 0, 0).Children(func() {
				// What can't be undone takes a click, or a tab to its
				// button: Enter, or a stray key after the one that asked,
				// cancels.
				cancel := ui.Button(c, "Cancel")
				if danger {
					cancel.AutoFocus()
				}
				if cancel.Clicked() {
					finish(false)
				}
				// What can't be undone is in red, its hover and press too.
				th := *tt
				if danger {
					red := ui.Hex("#cf222e") // white on it at 5:1 and more
					if tt.Dark {
						red = ui.Hex("#c43f39")
					}
					th.Accent = red
					th.AccentHover = red.Mix(ui.RGB(0, 0, 0), 0.12)
					th.AccentPressed = red.Mix(ui.RGB(0, 0, 0), 0.22)
				}
				ui.Themed(c, &th).Background(ui.Transparent).Children(func() {
					ok := ui.PrimaryButton(c, d.button)
					if !danger {
						ok.AutoFocus()
					}
					if ok.Clicked() {
						finish(true)
					}
				})
			})
		})
		if a.dialog == d && !d.open {
			finish(false)
		}
		return
	}
	sheet(420, func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, d.title).FontSize(14).FontWeight(600)
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
