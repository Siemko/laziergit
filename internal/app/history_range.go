package app

import (
	"fmt"

	"github.com/Siemko/laziergit/internal/core"
)

// History chooses a range of commits, as lazygit does: Shift with the keys
// that move, or a Shift-click, extends it from its other end, v makes the
// keys that move extend it until v again, and a move without either leaves
// the one commit moved to. Copying acts on the commits chosen; drop, fixup
// and move on the range, which must be commits next to each other. The
// range is the app's own, as the staging view's lines are: the list knows
// only the commit the keys are on, which its arrow keys and clicks move.

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

// anchorFrom is the end of the range a move from row cursor extends it
// from: its other end, or cursor, for a range of one.
func (a *App) anchorFrom(cursor int) int {
	lo, hi, ok := a.commitRange()
	switch {
	case !ok || lo == hi:
		return cursor
	case cursor == lo:
		return hi
	case cursor == hi:
		return lo
	}
	return cursor
}

// setCommitSel puts the keys on row i, the range from anchor to it.
func (a *App) setCommitSel(anchor, i int) {
	a.commitSel, a.commitSelSeen = i, i
	a.chooseCommits(anchor, i)
	a.commitsList.ScrollIntoView(i)
}

// extendCommits moves the keys d commits on, the range with them, held at
// its other end.
func (a *App) extendCommits(d int) {
	if n := len(a.snap.Commits); n > 0 {
		a.setCommitSel(a.anchorFrom(a.commitSel), max(0, min(a.commitSel+d, n-1)))
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

// followCommitSel follows a move of the list's own, by its arrow keys or a
// click: one to the commit moved to, or, with v or a Shift-click (on row
// shiftClicked, -1 for none), the range extended to it. It reports whether
// the range changed.
func (a *App) followCommitSel(shiftClicked int) bool {
	if shiftClicked >= 0 {
		// The list may take a Shift-click for a click, or leave it.
		a.commitSel = shiftClicked
	}
	if a.commitSel == a.commitSelSeen {
		return false
	}
	prev, i := a.commitSelSeen, a.commitSel
	a.commitSelSeen = i
	if shiftClicked == i || a.commitSticky {
		a.chooseCommits(a.anchorFrom(prev), i)
	} else {
		a.chooseCommits(i, i)
	}
	return true
}

// toggleCommitRange is v: j and k extend the range until v again.
func (a *App) toggleCommitRange() {
	a.commitSticky = !a.commitSticky
	if a.commitSticky && len(a.chosenCommits()) == 1 {
		a.chooseCommits(a.commitSel, a.commitSel)
	}
}

// loadCommitRange keeps the range to commits in the log loaded, or, after a
// move of one, chooses the rows it moved to, whose hashes are new.
func (a *App) loadCommitRange() {
	a.commitSelSeen = a.commitSel // clamped to the log loaded
	if m := a.commitsMoved; m[0] >= 0 {
		a.commitsMoved = [2]int{-1, -1}
		if m[1] < len(a.snap.Commits) {
			a.chooseCommits(m[0], m[1])
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
	all := true
	for _, i := range rows {
		all = all && a.isCopied(commits[i])
	}
	for _, i := range rows {
		if all || !a.isCopied(commits[i]) {
			a.toggleCopy("HEAD", commits, i)
		}
	}
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
		a.run("Dropping…", func(r *core.Repo) error { return r.DropCommits(commits, lo, hi) })
	})
}

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
	a.run("Fixing up…", func(r *core.Repo) error { return r.FixupCommits(commits, lo, hi) })
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
		a.commitSelSeen = a.commitSel
		a.commitsMoved = [2]int{lo + delta, hi + delta}
	})
}
