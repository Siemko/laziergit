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
		if headerButton(c, label).Disabled(len(files) == 0).Tooltip(label + " (a)").Clicked() {
			a.toggleAll()
		}
	})
	a.filesList.Selected = &a.fileSel
	a.filesList.Key = func(i int) any { return files[i].Path }
	a.filesList.Label = func(i int) string { return files[i].Path }
	hot := a.hot(c, paneList)
	var list ui.Element
	listIn(c, hot, func() {
		list = ui.List(c.Key("files"), &a.filesList, len(files), func(i int) {
			a.fileRow(c, pal, files[i], hot && i == a.fileSel)
		}).Grow(1).Padding(4, 6).FocusBind(&a.pane, paneList).FocusRing(false).Children(func() {
			if len(files) == 0 {
				emptyState(c, pal, iconCheck, "Working tree clean", "Nothing to commit.")
			}
		})
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
	ui.Row(c).Height(rowHeight).Padding(0, 2, 0, 8).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, letter).Font(codeFont).FontSize(11).FontWeight(700).TextColor(color).Width(14).TextAlign(ui.Center)
		fileName(c, pal, tt, f.Path, sel, 13)
		// The staged state: a check when all is staged, a bar when some.
		// It has a slot of its own, that the name never runs under.
		conflicted := f.HasUnstagedChanges && f.HasInlineMergeConflicts
		if conflicted {
			// Staged as its conflicts are resolved, not by hand.
			ui.Box(c).Size(24, 24).Shrink(0)
			return
		}
		slot := ui.ButtonBase(c.Key("stage")).Size(24, 24).Shrink(0).Center().Label("Toggle staged").Tooltip("Stage / unstage (space)")
		hovered := slot.Hovered()
		slot.Children(func() {
			box := ui.Box(c).Size(16, 16).Radius(4).Center()
			// The window's accent, though the list shows its choice in
			// gray; on the blue of a row chosen, ringed in white.
			white := ui.RGB(255, 255, 255)
			switch {
			case f.HasStagedChanges && !f.HasUnstagedChanges:
				box.Background(a.accent).TextColor(white).Children(func() { ui.Icon(c, iconCheck).FontSize(11) })
			case f.HasStagedChanges:
				box.Background(a.accent).Children(func() { ui.Box(c).Size(8, 2).Radius(1).Background(white) })
			default:
				border := tt.Border.Mix(tt.Text, 0.3)
				if hovered {
					border = a.accent
				}
				box.Background(t(c).Background).Border(1, border)
			}
			if sel && f.HasStagedChanges {
				box.Border(1, white.Alpha(0.8))
			}
		})
		if slot.Clicked() {
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
	if f.IsNestedRepository() {
		a.confirm("Delete the repository "+path.Base(f.Path)+"?", "It is a repository of its own: it is deleted with all of its history, which can't be undone.", "Delete Repository", func() {
			a.run("Deleting…", func(r *core.Repo) error { return r.DeleteNestedRepository(f) })
		})
		return
	}
	a.confirm("Discard changes?", f.Path+"\n\nIts changes are lost; this can't be undone.", "Discard", func() {
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
		ui.TextInput(c.Key("summary"), &a.summary).Placeholder("Summary").Label("Commit summary").Tooltip("Commit summary (c)").
			FocusBind(&a.pane, paneSummary)
		ui.TextArea(c.Key("description"), &a.description).Placeholder("Description").Label("Commit description").Height(64).
			FocusBind(&a.pane, paneDescription)
		// The button goes below the check box where the two don't fit side
		// by side, rather than being cut off.
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Justify(ui.End).Children(func() {
			ui.Checkbox(c, &a.amend, "Amend last commit").FontSize(12)
			ui.Spacer(c)
			label := "Commit"
			if a.amend {
				label = "Amend"
			}
			if defaultButton(c, label, !(!staged && !a.amend || a.summary == "" && !a.amend)).
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
	pathHeader(c, pal, f.Path, func() {
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
	pathHeader(c, pal, f.Path, nil)
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

// fileName is a file's name and, smaller, its folder: the name keeps its
// room as the row narrows, the folder giving way first, both ending in an
// ellipsis rather than running under what is beside them.
func fileName(c *ui.Context, pal *palette, tt *ui.Theme, p string, sel bool, size float32) {
	dir, name := path.Split(p)
	row := ui.Row(c.Key("name")).Gap(6).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Baseline).Clip().Tooltip(p)
	// The folder shows where a useful part of it fits beside the name.
	if room := row.Bounds().W; room > 0 && dir != "" {
		nameW, _ := c.MeasureText(0, ui.Span{Text: name, Size: size})
		if room-nameW-6 < 36 {
			dir = ""
		}
	}
	row.Children(func() {
		ui.Text(c, name).FontSize(size).SingleLine().Shrink(1).MinWidth(min(48, size*4))
		if dir != "" {
			ui.Text(c, strings.TrimSuffix(dir, "/")).FontSize(size - 2).TextColor(tint(sel, tt, pal.muted)).
				SingleLine().Shrink(1000).MinWidth(0)
		}
	})
}
