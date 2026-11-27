package app

import (
	"path/filepath"
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
	// History's actions act on the commits chosen, and say so.
	many := s != nil && len(a.chosenCommits()) > 1
	commits := "Commit"
	if many {
		commits = "Commits"
	}
	acts := []action{
		{title: "Go to Changes", mods: 0, key: ui.Key1, keyName: "1", enabled: true, run: func() { a.goTo(secChanges) }},
		{title: "Go to History", key: ui.Key2, keyName: "2", enabled: true, run: func() { a.goTo(secHistory) }},
		{title: "Go to Branches", key: ui.Key3, keyName: "3", enabled: true, run: func() { a.goTo(secBranches) }},
		{title: "Go to Stashes", key: ui.Key4, keyName: "4", enabled: true, run: func() { a.goTo(secStash) }},
		{title: "Go to Remote Branches", key: ui.Key5, keyName: "5", enabled: s != nil && len(s.Remotes) > 0, run: a.goToRemotes},
		{title: "Go to Tags", key: ui.Key6, keyName: "6", enabled: true, run: func() { a.goTo(secTags) }},
		{title: "Go to Reflog", key: ui.Key7, keyName: "7", enabled: true, run: func() { a.goTo(secReflog) }},
		{title: "Go to Worktrees", key: ui.Key8, keyName: "8", enabled: true, run: func() { a.goTo(secWorktrees) }},
		{title: "Go to Submodules", key: ui.Key9, keyName: "9", enabled: true, run: func() { a.goTo(secSubmodules) }},
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
		{title: "Reword Commit…", section: secHistory, key: ui.KeyR, keyName: "r", hint: true, enabled: commit != nil && !many,
			run: func() { a.reword(a.commitSel) }},
		{title: "Fixup Into Commit Below", section: secHistory, key: ui.KeyF, keyName: "f", hint: true,
			enabled: commit != nil && a.canFixupChosen(), run: a.fixupChosen},
		{title: "Drop " + commits + "…", section: secHistory, key: ui.KeyD, keyName: "d", hint: true, enabled: commit != nil, run: a.dropChosen},
		{title: "Move " + commits + " Down", section: secHistory, mods: ui.Ctrl, key: ui.KeyJ, keyName: "⌃j", hint: true, enabled: commit != nil,
			run: func() { a.moveChosen(1) }},
		{title: "Move " + commits + " Up", section: secHistory, mods: ui.Ctrl, key: ui.KeyK, keyName: "⌃k", hint: true, enabled: commit != nil,
			run: func() { a.moveChosen(-1) }},

		{title: "Copy " + commits + " (Cherry-Pick)", section: secHistory, mods: ui.Shift, key: ui.KeyC, keyName: "C", hint: true, enabled: commit != nil,
			run: a.copyCommits},
		{title: "Toggle Range Selection", section: secHistory, pane: paneList, key: ui.KeyV, keyName: "v", hint: true, enabled: commit != nil,
			run: a.toggleCommitRange},
		{title: "Clear Selection", section: secHistory, pane: paneList, key: ui.KeyEscape, keyName: "esc", hint: true, enabled: a.ranged(),
			run: a.clearCommitRange},
		{title: "Extend Down", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyJ, keyName: "J", enabled: commit != nil,
			run: func() { a.extendCommits(1) }},
		{title: "Extend Up", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyK, keyName: "K", enabled: commit != nil,
			run: func() { a.extendCommits(-1) }},
		{title: "Extend Down", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyDown, keyName: "⇧↓", enabled: commit != nil,
			run: func() { a.extendCommits(1) }},
		{title: "Extend Up", section: secHistory, pane: paneList, mods: ui.Shift, key: ui.KeyUp, keyName: "⇧↑", enabled: commit != nil,
			run: func() { a.extendCommits(-1) }},

		// Branches
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
	acts = append(acts, a.refActions()...)
	acts = append(acts, a.panelActions()...)
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
	if a.pane == paneFiles && refSection(a.section) {
		return &a.branchCommitSel, len(a.branchCommits), &a.branchCommitsList
	}
	if a.pane == paneFiles && filesSection(a.section) {
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
	case secRemotes:
		return &a.remoteBranchSel, len(a.remoteBranches()), &a.remoteBranchesList
	case secTags:
		return &a.tagSel, len(a.snap.Tags), &a.tagsList
	case secReflog:
		return &a.reflogSel, len(a.snap.Reflog), &a.reflogList
	case secWorktrees:
		return &a.worktreeSel, len(a.snap.Worktrees), &a.worktreesList
	case secSubmodules:
		return &a.submoduleSel, len(a.snap.Submodules), &a.submodulesList
	}
	return nil, 0, nil
}

func (a *App) focusRight() {
	switch {
	case a.pane == paneList && (filesSection(a.section) || refSection(a.section)):
		a.pane = paneFiles
	case a.pane == paneList && a.section == secChanges, a.pane == paneFiles && filesSection(a.section):
		a.pane = paneDiff
	}
}

func (a *App) focusLeft() {
	switch a.pane {
	case paneDiff:
		if filesSection(a.section) {
			a.pane = paneFiles
		} else {
			a.pane = paneList
		}
	case paneFiles, paneSummary, paneDescription:
		a.pane = paneList
	}
}

// refActions are the keys of the commits of a branch, a remote branch or
// a tag, while the keys are in them.
func (a *App) refActions() []action {
	var acts []action
	for _, s := range []section{secBranches, secRemotes, secTags} {
		back := "Back to " + sectionTitles[s]
		acts = append(acts,
			action{title: "Copy Commit (Cherry-Pick)", section: s, pane: paneFiles, mods: ui.Shift, key: ui.KeyC, keyName: "C", hint: true,
				enabled: a.branchCommitSel < len(a.branchCommits), run: func() {
					if name := a.refName(); name != "" {
						a.toggleCopy(name, a.branchCommits, a.branchCommitSel)
					}
				}},
			action{title: back, section: s, pane: paneFiles, key: ui.KeyEscape, keyName: "esc", hint: true, enabled: true,
				run: func() { a.pane = paneList }})
	}
	return acts
}

// panelActions are the keys of the remote branches, the tags, the reflog,
// the worktrees and the submodules.
func (a *App) panelActions() []action {
	if a.snap == nil {
		return nil
	}
	rb, tag, entry, wt, sm := a.selectedRemoteBranch(), a.selectedTag(), a.selectedReflog(), a.selectedWorktree(), a.selectedSubmodule()
	return []action{
		// Remote branches
		{title: "Checkout Remote Branch", section: secRemotes, key: ui.KeySpace, keyName: "space", hint: true, enabled: rb != nil,
			run: func() { a.checkoutRemoteBranch(rb) }},
		{title: "New Branch From Remote Branch…", section: secRemotes, key: ui.KeyN, keyName: "n", hint: true, enabled: rb != nil,
			run: func() { a.newBranchFrom(rb.FullName()) }},
		{title: "Merge Into Current Branch…", section: secRemotes, mods: ui.Shift, key: ui.KeyM, keyName: "M", hint: true, enabled: rb != nil,
			run: func() { a.mergeRef(rb.FullName()) }},
		{title: "Rebase Current Branch Onto…", section: secRemotes, key: ui.KeyR, keyName: "r", hint: true, enabled: rb != nil,
			run: func() { a.rebaseOntoRef(rb.FullName()) }},
		{title: "Delete Remote Branch…", section: secRemotes, key: ui.KeyD, keyName: "d", hint: true, enabled: rb != nil,
			run: func() { a.deleteRemoteBranch(rb) }},
		{title: "Fetch Remote", section: secRemotes, key: ui.KeyF, keyName: "f", hint: true, enabled: a.remote() != nil, run: a.fetchRemote},

		// Tags
		{title: "Checkout Tag…", section: secTags, key: ui.KeySpace, keyName: "space", hint: true, enabled: tag != nil,
			run: func() { a.checkoutTag(tag) }},
		{title: "New Tag…", section: secTags, key: ui.KeyN, keyName: "n", hint: true, enabled: true, run: func() { a.newTag("HEAD") }},
		{title: "Push Tag…", section: secTags, mods: ui.Shift, key: ui.KeyP, keyName: "P", hint: true, enabled: tag != nil && len(a.snap.Remotes) > 0,
			run: func() { a.pushTag(tag) }},
		{title: "Delete Tag…", section: secTags, key: ui.KeyD, keyName: "d", hint: true, enabled: tag != nil, run: func() { a.deleteTag(tag) }},
		{title: "Tag Commit…", section: secHistory, key: ui.KeyT, keyName: "t", enabled: a.selectedCommit() != nil,
			run: func() { a.newTag(a.selectedCommit().Hash()) }},

		// Reflog
		{title: "Checkout Commit…", section: secReflog, key: ui.KeySpace, keyName: "space", hint: true, enabled: entry != nil,
			run: func() { a.checkoutCommit(entry) }},
		{title: "New Branch From Here…", section: secReflog, key: ui.KeyN, keyName: "n", hint: true, enabled: entry != nil,
			run: func() { a.newBranchFrom(entry.Hash()) }},
		{title: "Reset Hard to Here…", section: secReflog, key: ui.KeyG, keyName: "g", hint: true, enabled: entry != nil,
			run: func() { a.resetTo(entry, core.ResetHard) }},
		{title: "Reset Soft to Here", section: secReflog, enabled: entry != nil, run: func() { a.resetTo(entry, core.ResetSoft) }},
		{title: "Reset Mixed to Here", section: secReflog, enabled: entry != nil, run: func() { a.resetTo(entry, core.ResetMixed) }},
		{title: "Copy Commit (Cherry-Pick)", section: secReflog, mods: ui.Shift, key: ui.KeyC, keyName: "C", hint: true, enabled: entry != nil,
			run: func() { a.toggleCopy("the reflog", a.snap.Reflog, a.reflogSel) }},

		// Worktrees
		{title: "Open Worktree in a Window", section: secWorktrees, key: ui.KeySpace, keyName: "space", hint: true,
			enabled: wt != nil && !wt.IsCurrent && !wt.IsPathMissing, run: func() { a.openRepository(wt.Path) }},
		{title: "New Worktree…", key: ui.KeyN, section: secWorktrees, keyName: "n", hint: true, enabled: true, run: a.newWorktree},
		{title: "New Worktree From Branch…", section: secBranches, key: ui.KeyW, keyName: "w", enabled: a.selectedBranch() != nil, run: a.newWorktree},
		{title: "Remove Worktree…", section: secWorktrees, key: ui.KeyD, keyName: "d", hint: true,
			enabled: wt != nil && !wt.IsMain && !wt.IsCurrent, run: func() { a.removeWorktree(wt) }},

		// Submodules
		{title: "Open Submodule in a Window", section: secSubmodules, key: ui.KeySpace, keyName: "space", hint: true,
			enabled: sm != nil && sm.Initialized(), run: func() { a.openRepository(filepath.Join(a.repo.Path, sm.Path)) }},
		{title: "Update Submodule", section: secSubmodules, key: ui.KeyU, keyName: "u", hint: true, enabled: sm != nil,
			run: func() { a.updateSubmodule(sm) }},
		{title: "Update All Submodules", section: secSubmodules, mods: ui.Shift, key: ui.KeyU, keyName: "U", hint: true,
			enabled: len(a.snap.Submodules) > 0, run: a.updateAllSubmodules},
		{title: "Add Submodule…", section: secSubmodules, key: ui.KeyN, keyName: "n", hint: true, enabled: true, run: a.addSubmodule},
		{title: "Remove Submodule…", section: secSubmodules, key: ui.KeyD, keyName: "d", hint: true, enabled: sm != nil,
			run: func() { a.removeSubmodule(sm) }},
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
	// The focused pane's actions take their keys first, then the
	// section's, as P pushes a tag in Tags, then those of every section.
	const (
		ownPane = iota
		ownSection
		everywhere
	)
	for _, pass := range []int{ownPane, ownSection, everywhere} {
		for _, act := range acts {
			switch {
			case (act.pane != paneNone) != (pass == ownPane), pass == ownPane && act.pane != a.pane:
				continue
			case pass == ownSection && act.section == "", pass == everywhere && act.section != "":
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
	case a.pane == paneFiles && refSection(a.section):
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
