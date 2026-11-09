package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// diffView shows the diff load returns, loading it off the UI thread when
// key changes. A diff loaded again, the same, keeps its scroll position.
func (a *App) diffView(c *ui.Context, pal *palette, key string, load func() (string, error)) {
	if key != a.diffKey {
		a.diffKey = key
		a.host.Go(func() {
			text, err := load()
			a.host.Post(func() {
				if a.diffKey != key {
					return
				}
				if err != nil {
					a.diff, a.diffEmpty = nil, err.Error()
					return
				}
				a.setDiff(text)
			})
		})
	}
	ui.Column(c.Key("diff")).Grow(1).MinHeight(0).FocusBind(&a.pane, paneDiff).FocusRing(false).Children(func() {
		if a.diff == nil {
			msg := a.diffEmpty
			if msg == "" {
				msg = "No changes to show"
			}
			emptyState(c, pal, iconChanges, msg, "")
			return
		}
		if a.unified == 1 {
			a.diff.Layout = editor.DiffSplit
		} else {
			a.diff.Layout = editor.DiffUnified
		}
		editor.DiffView(c, a.diff).Grow(1).FillWidth()
	})
}

func (a *App) setDiff(text string) {
	if a.diff != nil && a.diffText == text {
		return
	}
	a.diffText = text
	if strings.TrimSpace(text) == "" {
		a.diff, a.diffEmpty = nil, "No changes to show"
		return
	}
	files, err := editor.ParsePatch(text)
	if err != nil || len(files) == 0 {
		a.diff, a.diffEmpty = nil, "Binary file, or nothing to show"
		return
	}
	d := &editor.DiffState{
		Font:           ui.Font{Family: codeFont, Size: 12},
		KeepScrollbars: true,
		// No outline on brackets: the caret of a diff read is not shown.
		Theme: &editor.Theme{MatchingBracket: ui.RGBA(0, 0, 0, 0.004)},
	}
	d.SetPatch(files[0])
	a.diff, a.diffEmpty = d, ""
}

func (a *App) layoutToggle(c *ui.Context) {
	small(c, func() { ui.Segmented(c.Key("layout"), &a.unified, "Unified", "Split").Label("Diff layout") })
}
