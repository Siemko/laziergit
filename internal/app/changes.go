package app

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

func (a *App) selectedFile() *models.File {
	if a.snap == nil || a.fileSel < 0 || a.fileSel >= len(a.snap.Files) {
		return nil
	}
	return a.snap.Files[a.fileSel]
}

// statusLetter is the one letter a file's change shows as, and its color.
func statusLetter(pal *palette, f *models.File) (string, ui.Color) {
	switch {
	case f.HasMergeConflicts:
		return "!", pal.conflict
	case !f.Tracked && !f.HasStagedChanges:
		return "U", pal.added
	case f.Added:
		return "A", pal.added
	case f.Deleted:
		return "D", pal.deleted
	case f.IsRename():
		return "R", pal.renamed
	}
	return "M", pal.modified
}

func (a *App) changesList(c *ui.Context, pal *palette) {
	files := a.snap.Files
	staged := 0
	for _, f := range files {
		if f.HasStagedChanges && !f.HasUnstagedChanges {
			staged++
		}
	}
	paneHeader(c, pal, fmt.Sprintf("%s · %d staged", plural(len(files), "changed file"), staged), func() {
		label := "Stage All"
		if len(files) > 0 && staged == len(files) {
			label = "Unstage All"
		}
		if ui.Button(c, label).Disabled(len(files) == 0).Tooltip(label + " (a)").Clicked() {
			a.toggleAll()
		}
	})
	a.filesList.Selected = &a.fileSel
	a.filesList.Key = func(i int) any { return files[i].Path }
	a.filesList.Label = func(i int) string { return files[i].Path }
	list := ui.List(c.Key("files"), &a.filesList, len(files), func(i int) {
		a.fileRow(c, pal, files[i], i == a.fileSel)
	}).Grow(1).FocusBind(&a.pane, paneList).Children(func() {
		if len(files) == 0 {
			emptyState(c, pal, iconCheck, "Working tree clean", "Nothing to commit.")
		}
	})
	if list.Submitted() {
		a.pane = paneDiff
	}
	a.composer(c, pal)
}

func (a *App) fileRow(c *ui.Context, pal *palette, f *models.File, sel bool) {
	tt := c.Theme()
	letter, color := statusLetter(pal, f)
	color = tint(sel, tt, color)
	dir, name := path.Split(f.Path)
	ui.Row(c).Height(rowHeight).Padding(0, 10, 0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, letter).Font(codeFont).FontSize(11).FontWeight(700).TextColor(color).Width(14).TextAlign(ui.Center)
		ui.Row(c).Gap(6).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, name).FontSize(13).SingleLine().Shrink(0)
			if dir != "" {
				ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(11).TextColor(tint(sel, tt, pal.muted)).SingleLine().Shrink(1).MinWidth(0)
			}
		})
		// The staged state: a check when all is staged, a dash when some.
		conflicted := f.HasUnstagedChanges && f.HasInlineMergeConflicts
		box := ui.ButtonBase(c.Key("stage")).Size(16, 16).Radius(4).Center().Label("Toggle staged").Tooltip("Stage / unstage (space)")
		if conflicted {
			box.Disabled(true).Tooltip("Resolve its conflicts to stage it")
		}
		fill, mark := tt.Accent, tt.AccentText
		if sel {
			fill, mark = tt.AccentText, tt.Accent
		}
		switch {
		case f.HasStagedChanges && !f.HasUnstagedChanges:
			box.Background(fill).TextColor(mark).Children(func() { ui.Icon(c, iconCheck).FontSize(11) })
		case f.HasStagedChanges:
			box.Background(fill.Alpha(0.55)).TextColor(mark).Children(func() { ui.Text(c, "–").FontSize(11).Bold() })
		default:
			box.Border(1.2, tint(sel, tt, pal.muted))
		}
		if box.Clicked() && !conflicted {
			a.run("Staging…", func(r *core.Repo) error { return r.ToggleStaged(f) })
		}
	}).ContextMenu(func(m *ui.Menu) {
		label := "Stage"
		if !f.HasUnstagedChanges {
			label = "Unstage"
		}
		if m.Item(label).Shortcut(0, ui.KeySpace).Chosen() {
			a.run("Staging…", func(r *core.Repo) error { return r.ToggleStaged(f) })
		}
		m.Separator()
		if m.Item("Discard Changes…").Shortcut(0, ui.KeyD).Chosen() {
			a.confirmDiscard(f)
		}
	})
}

func (a *App) toggleAll() {
	files := a.snap.Files
	a.run("Staging…", func(r *core.Repo) error { return r.ToggleAllStaged(files) })
}

func (a *App) confirmDiscard(f *models.File) {
	a.confirm("Discard changes to "+f.Path+"?", "Its changes are lost; this can't be undone.", "Discard", func() {
		a.run("Discarding…", func(r *core.Repo) error { return r.Discard(f) })
	})
}

// composer is where the commit's message is written, under the files.
func (a *App) composer(c *ui.Context, pal *palette) {
	tt := c.Theme()
	staged := false
	for _, f := range a.snap.Files {
		if f.HasStagedChanges {
			staged = true
		}
	}
	ui.Column(c).Shrink(0).Padding(12).Gap(8).BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(tt.Background).Children(func() {
		ui.TextInput(c.Key("summary"), &a.summary).Placeholder("Summary (c)").Label("Commit summary").FocusBind(&a.pane, paneSummary)
		ui.TextArea(c.Key("description"), &a.description).Placeholder("Description").Label("Commit description").Height(64).
			FocusBind(&a.pane, paneDescription)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c, &a.amend, "Amend last commit")
			ui.Spacer(c)
			label := "Commit"
			if a.amend {
				label = "Amend"
			}
			if ui.PrimaryButton(c, label).Disabled(!staged && !a.amend || a.summary == "" && !a.amend).
				Tooltip(label + " (⌘↩)").Clicked() {
				a.commit()
			}
		})
	})
}

func (a *App) commit() {
	summary, desc, amend := strings.TrimSpace(a.summary), strings.TrimSpace(a.description), a.amend
	a.run("Committing…", func(r *core.Repo) error { return r.Commit(summary, desc, amend) }, func() {
		a.summary, a.description, a.amend = "", "", false
		a.pane = paneList
		a.toast("Committed", summary)
	})
}

func (a *App) fileDetail(c *ui.Context, pal *palette) {
	f := a.selectedFile()
	if f == nil {
		emptyState(c, pal, iconChanges, "No file chosen", "")
		return
	}
	if f.HasInlineMergeConflicts {
		a.conflictPane(c, pal, f)
		return
	}
	if f.HasMergeConflicts {
		a.conflictWithoutMarkers(c, pal, f)
		return
	}
	both := f.HasStagedChanges && f.HasUnstagedChanges
	staged := a.showingStaged(f)
	paneHeader(c, pal, f.Path, func() {
		if both {
			ui.Segmented(c, &a.diffStaged, "Unstaged", "Staged").Label("Which changes").Tooltip("Switch (tab)")
		}
	})
	a.stagingPane(c, pal, f, staged)
}

// showingStaged reports whether the diff of f shows its staged changes:
// those it has alone, or the ones chosen of both.
func (a *App) showingStaged(f *models.File) bool {
	if f.HasStagedChanges && f.HasUnstagedChanges {
		return a.diffStaged == 1
	}
	return f.HasStagedChanges
}

// conflictWithoutMarkers is a conflict git wrote no markers for, as a file
// one side deleted and the other changed.
func (a *App) conflictWithoutMarkers(c *ui.Context, pal *palette, f *models.File) {
	paneHeader(c, pal, f.Path, nil)
	ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
		ui.Icon(c, iconWarning).FontSize(28).TextColor(pal.conflict)
		ui.Text(c, "Conflict: "+conflictKind(f.ShortStatus)).FontSize(13).FontWeight(600)
		ui.Text(c, "Edit or delete the file as it should be, then mark it resolved.").FontSize(12).TextColor(pal.muted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "Mark as Resolved").Clicked() {
				a.markResolved(f)
			}
		})
	})
}

func conflictKind(status string) string {
	switch status {
	case "DU":
		return "deleted by us"
	case "UD":
		return "deleted by them"
	case "AA":
		return "added by both"
	case "AU":
		return "added by us"
	case "UA":
		return "added by them"
	case "DD":
		return "deleted by both"
	}
	return "changed by both"
}
