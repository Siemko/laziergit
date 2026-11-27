package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/core"
)

// History chooses a range of commits, as lazygit and Finder do. The list
// holds the commits chosen (commitsChosen, its Selection) and the one the
// keys are on (commitSel, its Selected), and takes the mouse: a click
// chooses one commit, a Shift-click extends the range to it, a Cmd-click
// adds a commit or takes it out, and a drag over the rows chooses those it
// goes over. The keys that move do as the mouse does: alone they choose
// the commit moved to, with Shift (or J and K) they extend the range, and
// v makes them extend it until v again. Esc leaves the one commit.
//
// A range extends from its anchor (commitAnchor, a hash), which the app
// keeps, rather than from the row the list last chose alone: j and k move
// the choice without the list. Copying acts on the commits chosen; drop,
// fixup and move on a range of commits next to each other.

// commitInput is the last input the list of History saw: a press of the
// pointer, with its clicks, or a key, with the modifiers held.
type commitInput struct {
	pointer bool
	button  int
	clicks  int
	mods    ui.Modifiers
	key     ui.Key
}

// observeCommitInput notes the input that comes to the list, for the
// change of the choice it makes to be told apart: a Shift-click from a
// click, a double click from Enter.
func (a *App) observeCommitInput(ev ui.InputEvent) {
	switch ev.Kind {
	case ui.InputPointerDown:
		a.commitIn = commitInput{pointer: true, button: ev.Button, clicks: ev.Clicks, mods: ev.Mods}
	case ui.InputKeyDown:
		a.commitIn = commitInput{key: ev.Key, mods: ev.Mods}
	}
}

// chosenCommits is the rows chosen in History, newest first: the range,
// or the commit the keys are on.
func (a *App) chosenCommits() []int {
	var rows []int
	for i, c := range a.snap.Commits {
		if a.commitsChosen.Has(c.Hash()) {
			rows = append(rows, i)
		}
	}
	if len(rows) == 0 && a.commitSel >= 0 && a.commitSel < len(a.snap.Commits) {
		rows = []int{a.commitSel}
	}
	return rows
}

// commitRange is chosenCommits as rows lo to hi; ok is false for none, or
// for commits not next to each other, as Cmd-click chooses.
func (a *App) commitRange() (lo, hi int, ok bool) {
	rows := a.chosenCommits()
	if len(rows) == 0 {
		return 0, 0, false
	}
	lo, hi = rows[0], rows[len(rows)-1]
	return lo, hi, hi-lo+1 == len(rows)
}

// chooseCommits makes rows from to to, either way, History's range.
func (a *App) chooseCommits(from, to int) {
	a.commitsChosen.Clear()
	for i := max(min(from, to), 0); i <= max(from, to) && i < len(a.snap.Commits); i++ {
		a.commitsChosen.Add(a.snap.Commits[i].Hash())
	}
}

// anchorRow is the row a range extends from: the anchor's, or, gone or
// not set, the commit the keys are on.
func (a *App) anchorRow() int {
	if a.commitAnchor != "" {
		for i, c := range a.snap.Commits {
			if c.Hash() == a.commitAnchor {
				return i
			}
		}
	}
	return a.commitSel
}

func (a *App) setAnchor(i int) {
	a.commitAnchor = ""
	if i >= 0 && i < len(a.snap.Commits) {
		a.commitAnchor = a.snap.Commits[i].Hash()
	}
}

// setCommitSel puts the keys on row i, the range from anchor to it.
func (a *App) setCommitSel(anchor, i int) {
	a.commitsList.Choose(i)
	a.chooseCommits(anchor, i)
	a.setAnchor(anchor)
	a.commitsList.ScrollIntoView(i)
}

// extendCommits moves the keys d commits on, the range with them, held at
// its anchor.
func (a *App) extendCommits(d int) {
	if n := len(a.snap.Commits); n > 0 {
		a.setCommitSel(a.anchorRow(), max(0, min(a.commitSel+d, n-1)))
	}
}

// stepCommits is j and k in History: a move to one commit, or, with v,
// an extension of the range.
func (a *App) stepCommits(d int) {
	if a.commitSticky {
		a.extendCommits(d)
		return
	}
	i := max(0, min(a.commitSel+d, len(a.snap.Commits)-1))
	a.setCommitSel(i, i)
}

// toggleCommitRange is v: j and k extend the range until v again.
func (a *App) toggleCommitRange() {
	a.commitSticky = !a.commitSticky
	if a.commitSticky && len(a.chosenCommits()) <= 1 {
		a.setCommitSel(a.commitSel, a.commitSel)
	}
}

// clearCommitRange leaves the one commit the keys are on, out of v.
func (a *App) clearCommitRange() {
	a.commitSticky = false
	if i := a.commitSel; i >= 0 && i < len(a.snap.Commits) {
		a.setCommitSel(i, i)
	}
}

// ranged reports whether History has a range to act on, or v holds one.
func (a *App) ranged() bool {
	return a.snap != nil && (a.commitSticky || len(a.chosenCommits()) > 1)
}

// followCommitList follows a change of the choice the list made, by the
// input it saw: it keeps the anchor where the list's choice puts it, and
// extends a range from the anchor where the list extended it from the row
// it chose alone last, which j and k may have moved from since. It
// reports whether it changed the choice.
func (a *App) followCommitList() bool {
	in, i := a.commitIn, a.commitSel
	n := len(a.snap.Commits)
	if i < 0 || i >= n {
		return false
	}
	rows := a.chosenCommits()
	single := len(rows) == 1 && rows[0] == i
	extend := func() bool {
		if lo, hi, ok := a.commitRange(); ok && min(lo, hi) == min(a.anchorRow(), i) && max(lo, hi) == max(a.anchorRow(), i) {
			return false
		}
		a.chooseCommits(a.anchorRow(), i)
		return true
	}
	switch {
	case in.mods&ui.Shift != 0 && (in.pointer && in.button == 0 || !in.pointer):
		// A Shift-click, a Shift-drag or Shift with the keys that move.
		return extend()
	case in.mods&ui.Cmd != 0:
		// A Cmd-click adds a commit or takes it out, a Cmd-drag adds
		// those it goes over; Cmd+A chooses all.
		if in.pointer {
			a.commitSticky = false
			a.setAnchor(i)
		}
	case in.pointer:
		// A click chooses one commit; a drag chooses those from the one
		// it pressed, the range's other end, to the one under it.
		a.commitSticky = false
		lo, hi, ok := a.commitRange()
		switch {
		case !ok || single:
			a.setAnchor(i)
		case i == lo:
			a.setAnchor(hi)
		default:
			a.setAnchor(lo)
		}
	case single && a.commitSticky:
		// The arrow keys, with v: as j and k.
		return extend()
	case single:
		a.setAnchor(i)
	}
	return false
}

// loadCommitRange keeps the range to commits in the log loaded, or, after a
// move of one, chooses the rows it moved to, whose hashes are new.
func (a *App) loadCommitRange() {
	n := len(a.snap.Commits)
	if m := a.commitsMoved; m[0] >= 0 {
		a.commitsMoved = [2]int{-1, -1}
		if m[1] < n {
			anchor := m[0]
			if a.commitSel == m[0] {
				anchor = m[1]
			}
			a.commitsList.Choose(a.commitSel)
			a.chooseCommits(m[0], m[1])
			a.setAnchor(anchor)
			return
		}
	}
	shown := map[string]bool{}
	for _, c := range a.snap.Commits {
		shown[c.Hash()] = true
	}
	var gone []string
	for h := range a.commitsChosen.All() {
		if !shown[h] {
			gone = append(gone, h)
		}
	}
	for _, h := range gone {
		a.commitsChosen.Remove(h)
	}
	if !shown[a.commitAnchor] {
		a.commitAnchor = ""
	}
}

// adjacent is commitRange, saying so when the commits chosen aren't next
// to each other.
func (a *App) adjacent() (lo, hi int, ok bool) {
	lo, hi, ok = a.commitRange()
	if !ok && len(a.chosenCommits()) > 0 {
		a.toasts = append(a.toasts, toastReq{title: "Choose commits next to each other", desc: "a rebase acts on a range", kind: "info"})
	}
	return lo, hi, ok
}

// copyCommits copies the commits chosen to cherry-pick, or, all copied
// already, takes them out of the copies.
func (a *App) copyCommits() {
	commits, rows := a.snap.Commits, a.chosenCommits()
	cp := a.copies()
	all := true
	for _, i := range rows {
		all = all && cp.has(commits[i].Hash())
	}
	if all {
		for _, i := range rows {
			cp.remove(commits[i].Hash())
		}
	} else {
		cp.add(a.copySource("HEAD"), commits, rows)
	}
	a.copiesChanged()
}

func (a *App) dropChosen() {
	lo, hi, ok := a.adjacent()
	switch {
	case !ok:
		return
	case lo == hi:
		a.drop(lo)
		return
	}
	commits := a.snap.Commits
	a.confirm(fmt.Sprintf("Drop %d commits?", hi-lo+1), "They are removed from the branch's history by an interactive rebase.", "Drop", func() {
		a.run("Dropping…", func(r *core.Repo) error { return r.DropCommits(commits, lo, hi) }, a.clearCommitRangeAfter)
	})
}

// clearCommitRangeAfter leaves v once the range's commits are gone.
func (a *App) clearCommitRangeAfter() { a.commitSticky = false }

// canFixupChosen reports whether the range has a commit below it to fix up
// into, and no merge.
func (a *App) canFixupChosen() bool {
	lo, hi, ok := a.commitRange()
	if !ok || hi+1 >= len(a.snap.Commits) {
		return false
	}
	for _, c := range a.snap.Commits[lo : hi+1] {
		if c.IsMerge() {
			return false
		}
	}
	return true
}

func (a *App) fixupChosen() {
	lo, hi, ok := a.adjacent()
	if !ok {
		return
	}
	commits := a.snap.Commits
	a.run("Fixing up…", func(r *core.Repo) error { return r.FixupCommits(commits, lo, hi) }, a.clearCommitRangeAfter)
}

// canMoveChosen reports whether the range can move one place by delta.
func (a *App) canMoveChosen(delta int) bool {
	lo, hi, ok := a.commitRange()
	return ok && lo+delta >= 0 && hi+delta < len(a.snap.Commits)
}

// moveChosen moves the range one place, and keeps it chosen there.
func (a *App) moveChosen(delta int) {
	lo, hi, ok := a.adjacent()
	commits := a.snap.Commits
	if !ok || lo+delta < 0 || hi+delta >= len(commits) {
		return
	}
	a.run("Moving…", func(r *core.Repo) error { return r.MoveCommits(commits, lo, hi, delta) }, func() {
		a.commitSel += delta
		a.commitsMoved = [2]int{lo + delta, hi + delta}
	})
}

// rangeBar says how many commits are chosen, and acts on them, under the
// list, while there is a range.
func (a *App) rangeBar(c *ui.Context, pal *palette) {
	if !a.ranged() {
		return
	}
	n := len(a.chosenCommits())
	_, _, adjacent := a.commitRange()
	ui.Row(c.Key("rangeBar")).Height(44).Shrink(0).Padding(0, 8, 0, 12).Gap(6).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(pal.header).Children(func() {
		label := plural(n, "commit")
		if a.commitSticky {
			label += " · range"
		}
		ui.Text(c, label).FontSize(12).FontWeight(600).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if headerButton(c, "Copy").Tooltip("Copy to cherry-pick (C)").Clicked() {
			a.copyCommits()
		}
		if headerButton(c, "Fixup").Disabled(!a.canFixupChosen()).Tooltip("Fix up into the commit below (f)").Clicked() {
			a.fixupChosen()
		}
		if iconTextButton(c, pal, "↑", "Move up (⌃k)", a.canMoveChosen(-1)).Clicked() {
			a.moveChosen(-1)
		}
		if iconTextButton(c, pal, "↓", "Move down (⌃j)", a.canMoveChosen(1)).Clicked() {
			a.moveChosen(1)
		}
		if headerButton(c, "Drop").Disabled(!adjacent).Tooltip("Drop (d)").Clicked() {
			a.dropChosen()
		}
		if iconTextButton(c, pal, "✕", "Clear selection (esc)", true).Clicked() {
			a.clearCommitRange()
		}
	})
}

// iconTextButton is a small square button showing a glyph.
func iconTextButton(c *ui.Context, pal *palette, glyph, tip string, enabled bool) ui.Element {
	tt := c.Theme()
	b := ui.ButtonBase(c.Key(tip)).Size(26, 24).Radius(6).Center().Shrink(0).TextColor(tt.Text).Label(tip).Tooltip(tip).Disabled(!enabled)
	switch {
	case !enabled:
		b.Opacity(0.35)
	case b.Pressed():
		b.Background(pal.selected)
	case b.Hovered():
		b.Background(pal.hover)
	}
	b.Children(func() { ui.Text(c, glyph).FontSize(13) })
	return b
}
