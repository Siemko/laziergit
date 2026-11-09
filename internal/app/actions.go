package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

// action is a command of the app: the palette lists it, its key runs it,
// and the status bar hints at it. Every command is declared once, here.
type action struct {
	title   string
	section section // "" for every section
	pane    pane    // paneNone for every pane; the pane's own come first
	mods    ui.Modifiers
	key     ui.Key
	keyName string // as the user reads it, lazygit's for single keys
	hint    bool   // shown in the status bar
	enabled bool
	run     func()
}

func (a *App) actions() []action {
	s := a.snap
	file, commit, branch, stash := a.selectedFile(), a.selectedCommit(), a.selectedBranch(), a.selectedStash()
	hasFiles := s != nil && len(s.Files) > 0
	acts := []action{
		{title: "Go to Changes", mods: 0, key: ui.Key1, keyName: "1", enabled: true, run: func() { a.goTo(secChanges) }},
		{title: "Go to History", key: ui.Key2, keyName: "2", enabled: true, run: func() { a.goTo(secHistory) }},
		{title: "Go to Branches", key: ui.Key3, keyName: "3", enabled: true, run: func() { a.goTo(secBranches) }},
		{title: "Go to Stashes", key: ui.Key4, keyName: "4", enabled: true, run: func() { a.goTo(secStash) }},
		{title: "Next Item", key: ui.KeyJ, keyName: "j", enabled: true, run: func() { a.step(1) }},
		{title: "Previous Item", key: ui.KeyK, keyName: "k", enabled: true, run: func() { a.step(-1) }},
		{title: "Focus Diff", key: ui.KeyL, keyName: "l", enabled: true, run: func() { a.focusRight() }},
		{title: "Focus List", key: ui.KeyH, keyName: "h", enabled: true, run: func() { a.focusLeft() }},

		{title: "Fetch", key: ui.KeyF, keyName: "f", hint: false, enabled: s != nil, section: secChanges, run: a.fetch},
		{title: "Fetch", key: ui.KeyF, keyName: "f", enabled: s != nil, section: secBranches, run: a.fetch},
		{title: "Pull", key: ui.KeyP, keyName: "p", enabled: s != nil, run: a.pull},
		{title: "Push", mods: ui.Shift, key: ui.KeyP, keyName: "P", enabled: s != nil, run: func() { a.push(false) }},
		{title: "Force Push (with lease)", enabled: s != nil, run: func() {
			a.confirm("Force push "+a.headName()+"?", "The remote branch is overwritten, if no one pushed to it since you fetched.", "Force Push", func() { a.push(true) })
		}},
		{title: "Refresh", mods: ui.Cmd, key: ui.KeyR, keyName: "⌘R", enabled: true, run: func() { a.refresh(false) }},
		{title: "Toggle Command Log", mods: ui.Cmd, key: ui.KeyL, keyName: "⌘L", enabled: true, run: func() { a.logOpen = !a.logOpen }},
		{title: "Open Repository…", enabled: true, run: a.openFolder},
		{title: "Toggle Split / Unified Diff", mods: ui.Alt, key: ui.KeyS, keyName: "⌥S", enabled: true, run: func() { a.unified = 1 - a.unified }},

		{title: "Undo", key: ui.KeyZ, keyName: "z", enabled: s != nil, run: func() { a.undo(false) }},
		{title: "Redo", mods: ui.Shift, key: ui.KeyZ, keyName: "Z", enabled: s != nil, run: func() { a.undo(true) }},
		{title: "Paste Copied Commits…", mods: ui.Shift, key: ui.KeyV, keyName: "V", enabled: len(a.copied) > 0, run: a.pasteCopied},
		{title: "Clear Copied Commits", enabled: len(a.copied) > 0, run: func() { a.copied, a.copiedFrom = nil, "" }},

		// Changes
		{title: "Stage / Unstage File", section: secChanges, key: ui.KeySpace, keyName: "space", hint: true,
			enabled: file != nil && !(file.HasUnstagedChanges && file.HasInlineMergeConflicts),
			run:     func() { a.run("Staging…", func(r *core.Repo) error { return r.ToggleStaged(file) }) }},
		{title: "Stage / Unstage All", section: secChanges, key: ui.KeyA, keyName: "a", hint: true, enabled: hasFiles, run: a.toggleAll},
		{title: "Discard File Changes…", section: secChanges, key: ui.KeyD, keyName: "d", hint: true, enabled: file != nil,
			run: func() { a.confirmDiscard(file) }},
		{title: "Write Commit Message", section: secChanges, key: ui.KeyC, keyName: "c", hint: true, enabled: true,
			run: func() { a.pane = paneSummary }},
		{title: "Commit", section: secChanges, mods: ui.Cmd, key: ui.KeyEnter, keyName: "⌘↩", enabled: hasFiles || a.amend, run: a.commit},
		{title: "Amend Last Commit", section: secChanges, mods: ui.Shift, key: ui.KeyA, keyName: "A", enabled: s != nil && len(s.Commits) > 0,
			run: func() {
				a.confirm("Amend the last commit with the staged changes?", "", "Amend", func() {
					a.run("Amending…", func(r *core.Repo) error { return r.Commit("", "", true) })
				})
			}},
		{title: "Stash All Changes", section: secChanges, key: ui.KeyS, keyName: "s", hint: true, enabled: hasFiles, run: a.stashAll},

		// History
		{title: "Reword Commit…", section: secHistory, key: ui.KeyR, keyName: "r", hint: true, enabled: commit != nil,
			run: func() { a.reword(a.commitSel) }},
		{title: "Fixup Into Commit Below", section: secHistory, key: ui.KeyF, keyName: "f", hint: true,
			enabled: commit != nil && a.canFixupChosen(), run: a.fixupChosen},
		{title: "Drop Commit…", section: secHistory, key: ui.KeyD, keyName: "d", hint: true, enabled: commit != nil, run: a.dropChosen},
		{title: "Move Commit Down", section: secHistory, mods: ui.Ctrl, key: ui.KeyJ, keyName: "⌃j", hint: true, enabled: commit != nil,
			run: func() { a.moveChosen(1) }},
		{title: "Move Commit Up", section: secHistory, mods: ui.Ctrl, key: ui.KeyK, keyName: "⌃k", hint: true, enabled: commit != nil,
			run: func() { a.moveChosen(-1) }},

		{title: "Copy Commit (Cherry-Pick)", section: secHistory, mods: ui.Shift, key: ui.KeyC, keyName: "C", hint: true, enabled: commit != nil,
			run: a.copyCommits},
		{title: "Toggle Range Selection", section: secHistory, pane: paneList, key: ui.KeyV, keyName: "v", hint: true, enabled: commit != nil,
			run: a.toggleCommitRange},
		{title: "Extend Down", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyJ, keyName: "J", enabled: commit != nil,
			run: func() { a.extendCommits(1) }},
		{title: "Extend Up", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyK, keyName: "K", enabled: commit != nil,
			run: func() { a.extendCommits(-1) }},
		{title: "Extend Down", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyDown, keyName: "⇧↓", enabled: commit != nil,
			run: func() { a.extendCommits(1) }},
		{title: "Extend Up", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyUp, keyName: "⇧↑", enabled: commit != nil,
			run: func() { a.extendCommits(-1) }},

		// Branches
		{title: "Copy Commit (Cherry-Pick)", section: secBranches, pane: paneFiles, mods: ui.Shift, key: ui.KeyC, keyName: "C", hint: true,
			enabled: a.branchCommitSel < len(a.branchCommits), run: func() {
				if b := a.selectedBranch(); b != nil {
					a.toggleCopy(b.Name, a.branchCommits, a.branchCommitSel)
				}
			}},
		{title: "Back to Branches", section: secBranches, pane: paneFiles, key: ui.KeyEscape, keyName: "esc", hint: true, enabled: true,
			run: func() { a.pane = paneList }},
		{title: "Checkout Branch", section: secBranches, key: ui.KeySpace, keyName: "space", hint: true, enabled: branch != nil,
			run: func() { a.checkout(branch) }},
		{title: "New Branch…", section: secBranches, key: ui.KeyN, keyName: "n", hint: true, enabled: true, run: a.newBranch},
		{title: "Merge Into Current Branch…", section: secBranches, mods: ui.Shift, key: ui.KeyM, keyName: "M", hint: true, enabled: branch != nil && !branch.Head,
			run: func() { a.merge(branch) }},
		{title: "Rebase Current Branch Onto…", section: secBranches, key: ui.KeyR, keyName: "r", hint: true, enabled: branch != nil && !branch.Head,
			run: func() { a.rebaseOnto(branch) }},
		{title: "Delete Branch…", section: secBranches, key: ui.KeyD, keyName: "d", hint: true, enabled: branch != nil && !branch.Head,
			run: func() { a.deleteBranch(branch) }},

		// Stash
		{title: "Pop Stash", section: secStash, key: ui.KeyG, keyName: "g", hint: true, enabled: stash != nil, run: func() { a.stashPop(stash) }},
		{title: "Apply Stash", section: secStash, key: ui.KeySpace, keyName: "space", hint: true, enabled: stash != nil,
			run: func() { a.run("Applying…", func(r *core.Repo) error { return r.StashApply(stash) }) }},
		{title: "Drop Stash…", section: secStash, key: ui.KeyD, keyName: "d", hint: true, enabled: stash != nil, run: func() { a.stashDrop(stash) }},
	}
	if file != nil && file.HasInlineMergeConflicts {
		acts = append(acts, a.conflictActions(file)...)
	} else {
		acts = append(acts, a.stagingActions(file)...)
	}
	if s != nil && s.State.Any() {
		noun := titleCase(a.operationNoun(s.State))
		acts = append(acts,
			action{title: "Continue " + noun, enabled: true, run: a.continueOperation},
			action{title: "Abort " + noun + "…", enabled: true, run: a.abortOperation})
		if canSkip(s.State) {
			acts = append(acts, action{title: "Skip Commit", enabled: true, run: a.skipOperation})
		}
	}
	return acts
}

func (a *App) goTo(s section) {
	a.section = s
	a.pane = paneList
}

// step moves the choice of the list with the keyboard, as j and k do.
func (a *App) step(d int) {
	sel, n, st := a.current()
	if sel == nil || n == 0 {
		return
	}
	if sel == &a.commitSel {
		a.stepCommits(d)
		return
	}
	*sel = max(0, min(*sel+d, n-1))
	st.ScrollIntoView(*sel)
}

// current is the choice, length and state of the list the keys move.
func (a *App) current() (*int, int, *ui.ListState) {
	if a.snap == nil {
		return nil, 0, nil
	}
	if a.pane == paneFiles && a.section == secBranches {
		return &a.branchCommitSel, len(a.branchCommits), &a.branchCommitsList
	}
	if a.pane == paneFiles {
		return &a.detailFileSel, len(a.detailFiles), &a.detailFilesList
	}
	switch a.section {
	case secChanges:
		return &a.fileSel, len(a.snap.Files), &a.filesList
	case secHistory:
		return &a.commitSel, len(a.snap.Commits), &a.commitsList
	case secBranches:
		return &a.branchSel, len(a.snap.Branches), &a.branchesList
	case secStash:
		return &a.stashSel, len(a.snap.Stash), &a.stashList
	}
	return nil, 0, nil
}

func (a *App) focusRight() {
	switch {
	case a.pane == paneList && (a.section == secHistory || a.section == secStash || a.section == secBranches):
		a.pane = paneFiles
	case a.pane == paneList && a.section == secChanges, a.pane == paneFiles && a.section != secBranches:
		a.pane = paneDiff
	}
}

func (a *App) focusLeft() {
	switch a.pane {
	case paneDiff:
		if a.section == secHistory || a.section == secStash || a.section == secBranches {
			a.pane = paneFiles
		} else {
			a.pane = paneList
		}
	case paneFiles, paneSummary, paneDescription:
		a.pane = paneList
	}
}

// shortcuts runs the action of a key pressed: single keys while the focus
// takes no text, as lazygit's, and those with Cmd wherever it is.
func (a *App) shortcuts(c *ui.Context) {
	if a.dialog != nil || a.palette {
		return
	}
	if c.Shortcut(ui.Cmd, ui.KeyK) || !c.FocusTakesText() && c.Shortcut(ui.Shift, ui.KeySlash) {
		a.openPalette()
		return
	}
	if c.FocusTakesText() && c.Shortcut(0, ui.KeyEscape) {
		a.pane = paneList
		return
	}
	textual := c.FocusTakesText()
	acts := a.actions()
	// The focused pane's actions take their keys first.
	for _, ownPane := range []bool{true, false} {
		for _, act := range acts {
			if (act.pane != paneNone) != ownPane || ownPane && act.pane != a.pane {
				continue
			}
			if act.keyName == "" || act.section != "" && act.section != a.section || !act.enabled {
				continue
			}
			if textual && act.mods&(ui.Cmd|ui.Alt) == 0 {
				continue
			}
			if c.Shortcut(act.mods, act.key) {
				act.run()
				return
			}
		}
	}
}

// stagingActions are the keys of the staging view, lazygit's.
func (a *App) stagingActions(f *models.File) []action {
	if f == nil {
		return nil
	}
	s := &a.stage
	staged := a.showingStaged(f)
	ready := s.path == f.Path && !s.empty()
	verb := "Stage"
	if staged {
		verb = "Unstage"
	}
	act := func(title string, mods ui.Modifiers, key ui.Key, name string, hint bool, run func()) action {
		return action{title: title, section: secChanges, pane: paneDiff, mods: mods, key: key, keyName: name, hint: hint, enabled: ready, run: run}
	}
	acts := []action{
		act(verb+" Selected Lines", 0, ui.KeySpace, "space", true, func() { a.applyChosen(f, staged, false) }),
		act("Select Hunk", 0, ui.KeyA, "a", true, s.selectHunk),
		act("Toggle Range Selection", 0, ui.KeyV, "v", true, s.toggleRange),
		act("Next Change", 0, ui.KeyJ, "j", false, func() { s.move(1, false) }),
		act("Previous Change", 0, ui.KeyK, "k", false, func() { s.move(-1, false) }),
		act("Next Change", 0, ui.KeyDown, "↓", false, func() { s.move(1, false) }),
		act("Previous Change", 0, ui.KeyUp, "↑", false, func() { s.move(-1, false) }),
		act("Extend Down", ui.Shift, ui.KeyJ, "J", false, func() { s.move(1, true) }),
		act("Extend Up", ui.Shift, ui.KeyK, "K", false, func() { s.move(-1, true) }),
		act("Extend Down", ui.Shift, ui.KeyDown, "⇧↓", false, func() { s.move(1, true) }),
		act("Extend Up", ui.Shift, ui.KeyUp, "⇧↑", false, func() { s.move(-1, true) }),
		act("Back to Files", 0, ui.KeyEscape, "esc", true, func() { a.pane = paneList }),
	}
	if f.HasStagedChanges && f.HasUnstagedChanges {
		acts = append(acts, act("Switch Staged / Unstaged", 0, ui.KeyTab, "tab", true, func() { a.diffStaged = 1 - a.diffStaged }))
	}
	if !staged {
		acts = append(acts, act("Discard Selected Lines…", 0, ui.KeyD, "d", true, func() { a.applyChosen(f, staged, true) }))
	}
	return acts
}

type hint struct{ key, label string }

func (a *App) hints() []hint {
	if a.pane == paneSummary || a.pane == paneDescription {
		// Typing: the keys that leave the text, not those that type.
		return []hint{{"⌘↩", "Commit"}, {"esc", "Back to Files"}, {"⌘K", "Commands"}}
	}
	var out []hint
	for _, act := range a.actions() {
		if act.hint && act.section == a.section && act.enabled && act.pane == paneForHints(a) {
			out = append(out, hint{act.keyName, strings.TrimSuffix(act.title, "…")})
		}
	}
	return append(out, hint{"?", "Commands"})
}

// paneForHints is the pane whose actions the status bar shows: the diff's
// own while it has the keys, the list's otherwise.
func paneForHints(a *App) pane {
	switch {
	case a.pane == paneDiff && a.section == secChanges:
		return paneDiff
	case a.pane == paneFiles && a.section == secBranches:
		return paneFiles
	}
	return paneNone
}

func (a *App) openPalette() {
	a.palette, a.query, a.paletteSel = true, "", 0
	a.paletteLst = ui.ListState{}
}

// paletteView is the command palette: every action, filtered as typed.
func (a *App) paletteView(c *ui.Context, pal *palette) {
	if !a.palette {
		return
	}
	var acts []action
	q := strings.ToLower(strings.TrimSpace(a.query))
	seen := map[string]bool{}
	for _, act := range a.actions() {
		if !act.enabled || act.section != "" && act.section != a.section || act.pane != paneNone && act.pane != a.pane || seen[act.title] {
			continue
		}
		seen[act.title] = true
		if q == "" || fuzzy(strings.ToLower(act.title), q) {
			acts = append(acts, act)
		}
	}
	a.paletteSel = max(0, min(a.paletteSel, len(acts)-1))
	runChosen := func(i int) {
		if i >= 0 && i < len(acts) {
			a.palette = false
			acts[i].run()
		}
	}
	tt := c.Theme()
	ui.DialogBase(c, &a.palette, func(back, panel ui.Element) {
		bg := pal.raised
		scrim := ui.RGBA(0, 0, 0, 0.18)
		if tt.Dark {
			scrim = ui.RGBA(0, 0, 0, 0.45)
		}
		_, h := c.Size()
		back.Background(scrim).Justify(ui.Start).Padding(min(120, h*0.16), 0, 0, 0)
		panel.Padding(8).Radius(12).Background(bg).Border(1, pal.border).Shadow(0, 18, 48, 0, ui.RGBA(0, 0, 0, 0.25))
		w, _ := c.Size()
		ui.Column(c).Width(min(560, w-64)).Gap(6).Children(func() {
			field := ui.SearchField(c.Key("query"), &a.query).Placeholder("Type a command").AutoFocus()
			if field.Changed() {
				a.paletteSel = 0
			}
			if c.Shortcut(0, ui.KeyDown) {
				a.paletteSel = min(a.paletteSel+1, len(acts)-1)
				a.paletteLst.ScrollIntoView(a.paletteSel)
			}
			if c.Shortcut(0, ui.KeyUp) {
				a.paletteSel = max(a.paletteSel-1, 0)
				a.paletteLst.ScrollIntoView(a.paletteSel)
			}
			if field.Submitted() || c.Shortcut(0, ui.KeyEnter) {
				runChosen(a.paletteSel)
			}
			a.paletteLst.Selected = &a.paletteSel
			list := ui.List(c.Key("commands"), &a.paletteLst, len(acts), func(i int) {
				act := acts[i]
				sel := i == a.paletteSel
				ui.Row(c).Height(32).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, act.title).FontSize(13).Grow(1).Shrink(1).MinWidth(0).SingleLine()
					if act.keyName != "" {
						cap := ui.Text(c, act.keyName).Font(codeFont).FontSize(11).Padding(1, 6).Radius(4).Shrink(0)
						if sel {
							cap.TextColor(c.Theme().AccentText).Background(c.Theme().AccentText.Alpha(0.2))
						} else {
							cap.TextColor(pal.muted).Background(pal.pill)
						}
					}
				})
			})
			if len(acts) == 0 {
				// The list, empty, takes no room: the note is centered in
				// its own.
				list.Height(0)
				ui.Box(c).Height(44).Center().Children(func() {
					ui.Text(c, "No matching commands").FontSize(12).TextColor(pal.muted)
				})
			} else {
				list.Height(min(float32(len(acts)), 10)*32 + 4)
			}
			if list.Submitted() {
				runChosen(a.paletteSel)
			}
		})
	})
}

// fuzzy reports whether the letters of q come in s in order.
func fuzzy(s, q string) bool {
	if strings.Contains(s, q) {
		return true
	}
	i := 0
	for _, r := range s {
		if i < len(q) && rune(q[i]) == r {
			i++
		}
	}
	return i == len(q)
}
