package app

import (
	"github.com/egoist/mygo/ui"
)

// palette is the app's colors over the theme's, in the light or the dark.
type palette struct {
	pane     ui.Color // behind the lists and the detail
	header   ui.Color // the bars over them
	border   ui.Color
	hover    ui.Color
	selected ui.Color // a row chosen while its list hasn't the focus
	pill     ui.Color
	added    ui.Color
	modified ui.Color
	deleted  ui.Color
	renamed  ui.Color
	conflict ui.Color
	hash     ui.Color
	unpushed ui.Color
	muted    ui.Color
	raised   ui.Color // dialogs, the palette and toasts, over the window
	graph    []ui.Color
}

var (
	lightPalette = palette{
		pane:     ui.Hex("#fafafa"),
		header:   ui.RGBA(255, 255, 255, 0.85),
		border:   ui.RGBA(17, 17, 17, 0.09),
		hover:    ui.RGBA(127, 127, 127, 0.09),
		selected: ui.RGBA(127, 127, 127, 0.16),
		pill:     ui.RGBA(127, 127, 127, 0.11),
		// Status colors dark enough to read at 11 points: 4.5:1 and more.
		added:    ui.Hex("#1a7f37"),
		modified: ui.Hex("#9a6700"),
		deleted:  ui.Hex("#cf222e"),
		renamed:  ui.Hex("#0969da"),
		conflict: ui.Hex("#a21caf"),
		hash:     ui.Hex("#9a6700"),
		unpushed: ui.Hex("#bf8700"), // the dot of a commit not pushed
		muted:    ui.RGBA(17, 17, 17, 0.6),
		raised:   ui.Hex("#ffffff"),
		graph: []ui.Color{
			ui.Hex("#3b82f6"), ui.Hex("#ec4899"), ui.Hex("#10b981"), ui.Hex("#f59e0b"),
			ui.Hex("#8b5cf6"), ui.Hex("#06b6d4"), ui.Hex("#ef4444"), ui.Hex("#84cc16"),
		},
	}
	darkPalette = palette{
		pane:     ui.Hex("#1c1c1f"), // a step above the detail's #18181b
		header:   ui.RGBA(30, 30, 32, 0.85),
		border:   ui.RGBA(255, 255, 255, 0.08),
		hover:    ui.RGBA(255, 255, 255, 0.06),
		selected: ui.RGBA(255, 255, 255, 0.11),
		pill:     ui.RGBA(255, 255, 255, 0.09),
		added:    ui.Hex("#3fb950"),
		modified: ui.Hex("#d29922"),
		deleted:  ui.Hex("#f85149"),
		renamed:  ui.Hex("#58a6ff"),
		conflict: ui.Hex("#db61a2"),
		hash:     ui.Hex("#d29922"),
		unpushed: ui.Hex("#d29922"),
		muted:    ui.RGBA(255, 255, 255, 0.55),
		raised:   ui.Hex("#232326"),
		graph: []ui.Color{
			ui.Hex("#60a5fa"), ui.Hex("#f472b6"), ui.Hex("#34d399"), ui.Hex("#fbbf24"),
			ui.Hex("#a78bfa"), ui.Hex("#22d3ee"), ui.Hex("#f87171"), ui.Hex("#a3e635"),
		},
	}
)

func paletteFor(t *ui.Theme) *palette {
	if t.Dark {
		return &darkPalette
	}
	return &lightPalette
}

const (
	codeFont       = "SF Mono, Menlo, monospace"
	titleBarHeight = 52
	rowHeight      = 28
	headerHeight   = 36 // the bars over the panes
)
