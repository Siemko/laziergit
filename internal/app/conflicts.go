package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

// contextShown is how many unchanged lines show around a conflict before
// the rest between conflicts fold.
const contextShown = 3

// conflictView is a file with merge conflicts: its conflicts as cards, each
// side kept with a click or a key, and the lines between them folded.
type conflictView struct {
	key     string // the file and snapshot the content is of
	path    string
	content string
	file    *core.ConflictedFile
	markers int // the file's conflict-marker-size

	// sel is the side chosen: an index into sides().
	sel  int
	undo []string

	items    []conflictItem
	expanded map[int]bool // folds opened, by their first line
	list     ui.ListState

	spans [][]ui.Span
	dark  bool

	// notText is set for a conflicted path that isn't a regular file, as a
	// symlink, which has no markers to resolve here.
	notText bool
}

// conflictItem is a row of the view: some unchanged lines, a fold of
// them, or a conflict.
type conflictItem struct {
	conflict int // -1 for lines
	from, to int // the lines, [from, to)
	fold     bool
}

// conflictSide is a side of a conflict the keys choose.
type conflictSide struct {
	conflict int
	pick     core.Pick
}

// set shows content: the app's own resolution, or the file as loaded,
// which, changed by another app, forgets the resolutions to undo.
func (v *conflictView) set(path, content string, markers int, ours bool) {
	v.notText = false
	if path == v.path && content == v.content {
		return
	}
	if path != v.path {
		v.sel, v.undo, v.expanded, v.list = 0, nil, map[int]bool{}, ui.ListState{}
	} else if !ours {
		v.undo = nil
	}
	v.path, v.content, v.markers = path, content, markers
	v.file = core.ParseConflicts(content, markers)
	v.spans = nil
	v.layout()
	v.sel = max(0, min(v.sel, len(v.sides())-1))
}

// layout splits the file into rows: the conflicts, and the lines between
// them, of which only those near a conflict show unless opened.
func (v *conflictView) layout() {
	v.items = v.items[:0]
	lines := func(from, to int, first, last bool) {
		if from >= to {
			return
		}
		head, tail := contextShown, contextShown
		if first {
			head = 0
		}
		if last {
			tail = 0
		}
		if to-from < head+tail+4 || v.expanded[from] {
			v.items = append(v.items, conflictItem{conflict: -1, from: from, to: to})
			return
		}
		if head > 0 {
			v.items = append(v.items, conflictItem{conflict: -1, from: from, to: from + head})
		}
		v.items = append(v.items, conflictItem{conflict: -1, from: from + head, to: to - tail, fold: true})
		if tail > 0 {
			v.items = append(v.items, conflictItem{conflict: -1, from: to - tail, to: to})
		}
	}
	at := 0
	for i, c := range v.file.Conflicts {
		lines(at, c.Start, i == 0, false)
		v.items = append(v.items, conflictItem{conflict: i, from: c.Start, to: c.End + 1})
		at = c.End + 1
	}
	lines(at, len(v.file.Lines), len(v.file.Conflicts) == 0, true)
}

// sides is every side the keys go through, conflict by conflict.
func (v *conflictView) sides() []conflictSide {
	var out []conflictSide
	for i, c := range v.file.Conflicts {
		out = append(out, conflictSide{i, core.PickOurs})
		if c.Base >= 0 {
			out = append(out, conflictSide{i, core.PickBase})
		}
		out = append(out, conflictSide{i, core.PickTheirs})
	}
	return out
}

func (v *conflictView) current() (conflictSide, bool) {
	sides := v.sides()
	if v.sel < 0 || v.sel >= len(sides) {
		return conflictSide{}, false
	}
	return sides[v.sel], true
}

func (v *conflictView) move(d int) {
	v.sel = max(0, min(v.sel+d, len(v.sides())-1))
	v.reveal()
}

// jump goes to the first side of the next or previous conflict.
func (v *conflictView) jump(d int) {
	cur, ok := v.current()
	if !ok {
		return
	}
	target := max(0, min(cur.conflict+d, len(v.file.Conflicts)-1))
	for i, s := range v.sides() {
		if s.conflict == target {
			v.sel = i
			break
		}
	}
	v.reveal()
}

func (v *conflictView) reveal() {
	cur, ok := v.current()
	if !ok {
		return
	}
	for i, it := range v.items {
		if it.conflict == cur.conflict {
			v.list.ScrollIntoView(i)
		}
	}
}

func (v *conflictView) highlight(c *ui.Context) {
	tt := c.Theme()
	if v.spans != nil && v.dark == tt.Dark {
		return
	}
	v.dark = tt.Dark
	et := editor.DefaultTheme(tt)
	tok := tokenizerFor(v.path)
	v.spans = make([][]ui.Span, len(v.file.Lines))
	var state uint64
	for i, l := range v.file.Lines {
		v.spans[i], state = tokenSpans(tok, et, expandTabs(strings.TrimRight(l, "\r\n")), state)
	}
}

// conflictPane shows the conflicts of f, loading its content when the file
// or the repository changes.
func (a *App) conflictPane(c *ui.Context, pal *palette, f *models.File) {
	v := &a.conflicts
	key := fmt.Sprintf("%d:%s", a.gen, f.Path)
	if key != v.key {
		v.key = key
		repo := a.repo
		a.host.Go(func() {
			content, err := repo.ReadConflicted(f.Path)
			a.host.Post(func() {
				if v.key != key {
					return
				}
				if errors.Is(err, core.ErrNotRegular) {
					v.path, v.file, v.notText = f.Path, nil, true
					return
				}
				if err != nil {
					a.toastErr(err)
					return
				}
				v.set(f.Path, content, f.ConflictMarkerSize, false)
			})
		})
	}
	if v.path == f.Path && v.notText {
		a.conflictWithoutMarkers(c, pal, f)
		return
	}
	if v.path != f.Path || v.file == nil {
		return
	}
	n := len(v.file.Conflicts)
	paneHeader(c, pal, f.Path, func() {
		ui.Text(c, plural(n, "conflict")+" left").FontSize(12).TextColor(pal.conflict).FontWeight(600)
		if ui.Button(c, "Undo").Disabled(len(v.undo) == 0).Tooltip("Undo (z)").Clicked() {
			a.undoConflict(f)
		}
		if ui.Button(c, "All Current").Disabled(n == 0).Tooltip("Keep the current side of every conflict").Clicked() {
			a.resolveConflict(f, -1, core.PickOurs)
		}
		if ui.Button(c, "All Incoming").Disabled(n == 0).Tooltip("Keep the incoming side of every conflict").Clicked() {
			a.resolveConflict(f, -1, core.PickTheirs)
		}
	})
	if n == 0 {
		ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
			ui.Icon(c, iconCheck).FontSize(28).TextColor(pal.added)
			ui.Text(c, "No conflict markers left").FontSize(13).FontWeight(600)
			ui.Text(c, "Mark the file resolved to stage it.").FontSize(12).TextColor(pal.muted)
			if ui.PrimaryButton(c, "Mark as Resolved").Clicked() {
				a.markResolved(f)
			}
		})
		return
	}
	v.highlight(c)
	ui.List(c.Key("conflicts"), &v.list, len(v.items), func(i int) {
		it := v.items[i]
		switch {
		case it.conflict >= 0:
			a.conflictCard(c, pal, f, it.conflict)
		case it.fold:
			a.conflictFold(c, pal, it)
		default:
			for l := it.from; l < it.to; l++ {
				a.codeLine(c, pal, v, l, ui.Color{})
			}
		}
	}).Grow(1).Padding(0, 0, 12, 0).FocusBind(&a.pane, paneDiff).Label("Conflicts")
}

func (a *App) codeLine(c *ui.Context, pal *palette, v *conflictView, line int, bg ui.Color) {
	ui.Row(c).Height(20).AlignItems(ui.Center).Background(bg).Children(func() {
		ui.Text(c, fmt.Sprint(line+1)).Width(44).Shrink(0).TextAlign(ui.End).Padding(0, 10, 0, 0).
			Font(codeFont).FontSize(11).TextColor(pal.muted)
		ui.RichText(c, v.spans[line]...).Font(codeFont).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
	})
}

func (a *App) conflictFold(c *ui.Context, pal *palette, it conflictItem) {
	v := &a.conflicts
	b := ui.ButtonBase(c.Key(fmt.Sprint("fold", it.from))).Height(26).Padding(0, 14).Gap(6).AlignItems(ui.Center).
		Background(pal.pill).Label("Show unchanged lines")
	if b.Hovered() {
		b.Background(pal.selected)
	}
	b.Children(func() {
		ui.Text(c, "⋯").FontSize(12).TextColor(pal.muted)
		ui.Text(c, plural(it.to-it.from, "unchanged line")).FontSize(11).TextColor(pal.muted)
	})
	if b.Clicked() {
		v.expanded[it.from] = true
		v.layout()
	}
}

// conflictCard is a conflict: its sides, each with the button keeping it.
func (a *App) conflictCard(c *ui.Context, pal *palette, f *models.File, i int) {
	tt := c.Theme()
	v := &a.conflicts
	cf := v.file.Conflicts[i]
	cur, _ := v.current()
	chosen := cur.conflict == i
	focused := a.pane == paneDiff
	border := pal.border
	if chosen {
		border = tt.Accent.Alpha(0.7)
	}
	ui.Column(c).Margin(8, 12, 8, 12).Radius(10).Border(1.5, border).Clip().Background(tt.Background).Children(func() {
		ui.Row(c).Height(34).Padding(0, 10, 0, 14).Gap(8).AlignItems(ui.Center).Background(pal.pill).Children(func() {
			ui.Icon(c, iconWarning).FontSize(13).TextColor(pal.conflict)
			ui.Textf(c, "Conflict %d of %d", i+1, len(v.file.Conflicts)).FontSize(12).FontWeight(600)
			ui.Spacer(c)
			if smallButton(c, pal, "Accept Both", false).Tooltip("Current, then incoming (b)").Clicked() {
				a.resolveConflict(f, i, core.PickBoth)
			}
		})
		side := func(pick core.Pick, title, label string, color ui.Color, from, to int) {
			sel := chosen && cur.pick == pick
			head := ui.Row(c.Key(fmt.Sprint("side", pick))).Height(30).Padding(0, 10, 0, 14).Gap(8).AlignItems(ui.Center).
				BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(color.Alpha(0.10))
			head.Children(func() {
				ui.Box(c).Size(8, 8).Radius(4).Background(color)
				ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(color)
				if label != "" {
					ui.Text(c, label).Font(codeFont).FontSize(11).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
				}
				ui.Spacer(c)
				if sel && focused {
					ui.Text(c, "space").Font(codeFont).FontSize(10).Padding(1, 5).Radius(4).Background(pal.pill).TextColor(pal.muted)
				}
				if smallButton(c, pal, "Accept "+title, sel).Clicked() {
					a.resolveConflict(f, i, pick)
				}
			})
			if head.Clicked() {
				a.pane = paneDiff
				for j, s := range v.sides() {
					if s.conflict == i && s.pick == pick {
						v.sel = j
					}
				}
			}
			bg := color.Alpha(0.06)
			if sel {
				bg = color.Alpha(0.16)
			}
			ui.Column(c).Padding(4, 0).Background(bg).Children(func() {
				if from == to {
					ui.Text(c, "(nothing)").FontSize(11).TextColor(pal.muted).Padding(2, 54)
				}
				for l := from; l < to; l++ {
					a.codeLine(c, pal, v, l, ui.Color{})
				}
			})
		}
		of, ot := cf.Ours()
		side(core.PickOurs, "Current", cf.OursLabel, pal.renamed, of, ot)
		if cf.Base >= 0 {
			bf, bt := cf.BaseLines()
			side(core.PickBase, "Base", "", pal.muted, bf, bt)
		}
		tf, tt2 := cf.Theirs()
		side(core.PickTheirs, "Incoming", cf.TheirsLabel, pal.conflict, tf, tt2)
	})
}

func smallButton(c *ui.Context, pal *palette, label string, primary bool) ui.Element {
	tt := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Height(22).Padding(0, 9).Radius(5).Center().Label(label).Shrink(0)
	switch {
	case primary:
		b.Background(tt.Accent).TextColor(tt.AccentText)
	case b.Hovered():
		b.Background(pal.hover).Border(1, pal.border)
	default:
		b.Background(tt.Background).Border(1, pal.border)
	}
	b.Children(func() { ui.Text(c, label).FontSize(11).FontWeight(500) })
	return b
}

// resolveConflict keeps a side of conflict i, or of all of them for -1,
// writing the file; with no conflict left, the file is staged.
func (a *App) resolveConflict(f *models.File, i int, pick core.Pick) {
	v := &a.conflicts
	if v.file == nil || a.busy > 0 {
		return // one resolution at a time, each from the file as written
	}
	content := v.file.ResolveAll(pick)
	if i >= 0 {
		content = v.file.Resolve(i, pick)
	}
	a.writeConflicted(f, v.content, content, true)
}

func (a *App) undoConflict(f *models.File) {
	v := &a.conflicts
	if len(v.undo) == 0 || a.busy > 0 {
		return
	}
	a.writeConflicted(f, v.content, v.undo[len(v.undo)-1], false)
}

// writeConflicted writes a resolution of f in place of expected, the
// content shown, and shows it once written: with no conflict left, the
// file is staged. A file changed since it was shown is loaded again,
// rather than overwritten.
func (a *App) writeConflicted(f *models.File, expected, content string, undoable bool) {
	v := &a.conflicts
	path, markers := f.Path, f.ConflictMarkerSize
	staged := false
	stale := false
	a.run("Resolving…", func(r *core.Repo) error {
		var err error
		staged, err = r.ResolveConflict(f, expected, content)
		if errors.Is(err, core.ErrStale) {
			stale = true
			return nil
		}
		return err
	}, func() {
		if v.path != path {
			// Another file shows now: it loads this one again when shown.
			if staged {
				a.toast("Resolved "+path, "")
				a.offerContinue = true
			}
			return
		}
		if stale {
			v.key = "" // load it again
			a.toasts = append(a.toasts, toastReq{title: path + " changed on disk", desc: "It was loaded again: choose again.", kind: "warning"})
			return
		}
		if undoable {
			v.undo = append(v.undo, expected)
		} else if len(v.undo) > 0 {
			v.undo = v.undo[:len(v.undo)-1]
		}
		v.set(path, content, markers, true)
		if staged {
			v.undo = nil
			a.toast("Resolved "+path, "")
			a.offerContinue = true
		}
	})
}

func (a *App) markResolved(f *models.File) {
	a.run("Staging…", func(r *core.Repo) error { return r.MarkResolved(f) }, func() { a.offerContinue = true })
}

// offerToContinue asks to continue the operation in progress once no file
// has conflicts left, as lazygit does.
func (a *App) offerToContinue() {
	if !a.offerContinue || a.loading || a.busy > 0 || a.snap == nil || a.dialog != nil {
		return
	}
	a.offerContinue = false
	st := a.snap.State
	if st.None() {
		return
	}
	for _, f := range a.snap.Files {
		if f.HasMergeConflicts {
			return
		}
	}
	noun := a.operationNoun(st)
	a.confirm("All conflicts are resolved", "Continue the "+noun+"?", "Continue "+titleCase(noun), func() {
		a.run("Continuing…", func(r *core.Repo) error { return r.ContinueOperation(st) })
	})
}

// conflictActions are the keys of the conflict view.
func (a *App) conflictActions(f *models.File) []action {
	v := &a.conflicts
	ready := v.path == f.Path && v.file != nil && len(v.file.Conflicts) > 0
	act := func(title string, mods ui.Modifiers, key ui.Key, name string, hint bool, run func()) action {
		return action{title: title, section: secChanges, pane: paneDiff, mods: mods, key: key, keyName: name, hint: hint, enabled: ready, run: run}
	}
	pick := func() {
		if cur, ok := v.current(); ok {
			a.resolveConflict(f, cur.conflict, cur.pick)
		}
	}
	both := func() {
		if cur, ok := v.current(); ok {
			a.resolveConflict(f, cur.conflict, core.PickBoth)
		}
	}
	acts := []action{
		act("Accept Chosen Side", 0, ui.KeySpace, "space", true, pick),
		act("Accept Both Sides", 0, ui.KeyB, "b", true, both),
		act("Next Side", 0, ui.KeyJ, "j", false, func() { v.move(1) }),
		act("Previous Side", 0, ui.KeyK, "k", false, func() { v.move(-1) }),
		act("Next Side", 0, ui.KeyDown, "↓", false, func() { v.move(1) }),
		act("Previous Side", 0, ui.KeyUp, "↑", false, func() { v.move(-1) }),
		act("Next Conflict", 0, ui.KeyRight, "→", true, func() { v.jump(1) }),
		act("Previous Conflict", 0, ui.KeyLeft, "←", true, func() { v.jump(-1) }),
		act("Back to Files", 0, ui.KeyEscape, "esc", true, func() { a.pane = paneList }),
	}
	undo := act("Undo Resolution", 0, ui.KeyZ, "z", true, func() { a.undoConflict(f) })
	undo.enabled = v.path == f.Path && len(v.undo) > 0
	return append(acts, undo)
}
