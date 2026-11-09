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
	graph    []ui.Color
}

var (
	lightPalette = palette{
		pane:     ui.Hex("#fbfbfa"),
		header:   ui.RGBA(255, 255, 255, 0.85),
		border:   ui.RGBA(17, 17, 17, 0.09),
		hover:    ui.RGBA(127, 127, 127, 0.09),
		selected: ui.RGBA(127, 127, 127, 0.16),
		pill:     ui.RGBA(127, 127, 127, 0.11),
		added:    ui.Hex("#1f9d55"),
		modified: ui.Hex("#c27c0e"),
		deleted:  ui.Hex("#d64045"),
		renamed:  ui.Hex("#3b82f6"),
		conflict: ui.Hex("#c026d3"),
		hash:     ui.Hex("#b45309"),
		unpushed: ui.Hex("#dc2626"),
		muted:    ui.RGBA(17, 17, 17, 0.5),
		graph: []ui.Color{
			ui.Hex("#3b82f6"), ui.Hex("#ec4899"), ui.Hex("#10b981"), ui.Hex("#f59e0b"),
			ui.Hex("#8b5cf6"), ui.Hex("#06b6d4"), ui.Hex("#ef4444"), ui.Hex("#84cc16"),
		},
	}
	darkPalette = palette{
		pane:     ui.Hex("#18181a"),
		header:   ui.RGBA(30, 30, 32, 0.85),
		border:   ui.RGBA(255, 255, 255, 0.08),
		hover:    ui.RGBA(255, 255, 255, 0.06),
		selected: ui.RGBA(255, 255, 255, 0.11),
		pill:     ui.RGBA(255, 255, 255, 0.09),
		added:    ui.Hex("#4ade80"),
		modified: ui.Hex("#fbbf24"),
		deleted:  ui.Hex("#f87171"),
		renamed:  ui.Hex("#60a5fa"),
		conflict: ui.Hex("#e879f9"),
		hash:     ui.Hex("#f0b458"),
		unpushed: ui.Hex("#f87171"),
		muted:    ui.RGBA(255, 255, 255, 0.5),
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
)
