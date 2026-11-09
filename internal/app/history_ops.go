package app

import (
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"laziergit/internal/core"
)

// copiedCommit is a commit copied to cherry-pick, with its place in the
// log it was copied from, so that the copies keep the log's order.
type copiedCommit struct {
	commit *models.Commit
	index  int
}

func copyLabel(copied bool) string {
	if copied {
		return "Uncopy Commit"
	}
	return "Copy Commit (Cherry-Pick)"
}

func (a *App) isCopied(c *models.Commit) bool {
	for _, cc := range a.copied {
		if cc.commit.Hash() == c.Hash() {
			return true
		}
	}
	return false
}

// toggleCopy copies commit i of a log to cherry-pick, or takes it out of
// the copies. Copying from another log starts the copies afresh.
func (a *App) toggleCopy(source string, commits []*models.Commit, i int) {
	if i < 0 || i >= len(commits) {
		return
	}
	if source != a.copiedFrom {
		a.copied, a.copiedFrom = nil, source
	}
	c := commits[i]
	for j, cc := range a.copied {
		if cc.commit.Hash() == c.Hash() {
			a.copied = slices.Delete(a.copied, j, j+1)
			return
		}
	}
	a.copied = append(a.copied, copiedCommit{c, i})
}

// pasteCopied cherry-picks the copies onto HEAD, after asking.
func (a *App) pasteCopied() {
	if len(a.copied) == 0 {
		return
	}
	copies := slices.Clone(a.copied)
	slices.SortFunc(copies, func(x, y copiedCommit) int { return x.index - y.index })
	commits := make([]*models.Commit, len(copies))
	for i, cc := range copies {
		commits[i] = cc.commit
	}
	a.confirm("Cherry-pick "+plural(len(commits), "commit")+" onto "+a.headName()+"?",
		"They are applied oldest first, as new commits.", "Cherry-Pick", func() {
			a.run("Cherry-picking…", func() error { return a.repo.CherryPick(commits) }, func() {
				a.copied, a.copiedFrom = nil, ""
				a.toast("Cherry-picked "+plural(len(commits), "commit"), "")
			})
		})
}

// copiedBanner shows the commits copied, and pastes them.
func (a *App) copiedBanner(c *ui.Context, pal *palette) {
	if len(a.copied) == 0 {
		return
	}
	ui.Row(c.Key("copiedBanner")).Padding(7, 14).Gap(10).AlignItems(ui.Center).Shrink(0).Background(pal.conflict.Alpha(0.10)).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Icon(c, iconCopy).FontSize(14).TextColor(pal.conflict)
		from := ""
		if a.copiedFrom != "HEAD" {
			from = " from " + a.copiedFrom
		}
		ui.Text(c, plural(len(a.copied), "commit")+" copied"+from).FontWeight(600).SingleLine().Shrink(1)
		ui.Spacer(c)
		if ui.Button(c, "Clear").Clicked() {
			a.copied, a.copiedFrom = nil, ""
		}
		if ui.PrimaryButton(c, "Paste onto "+a.headName()).Tooltip("Cherry-pick them (V)").Clicked() {
			a.pasteCopied()
		}
	})
}

// undo finds what undo (or redo) would do, as lazygit's z and Z, and asks
// before doing it.
func (a *App) undo(redo bool) {
	if a.repo == nil {
		return
	}
	repo := a.repo
	files := a.snap.Files
	a.host.Go(func() {
		step, err := repo.FindUndo(redo)
		a.host.Post(func() {
			if err != nil {
				a.toasts = append(a.toasts, toastReq{title: err.Error(), kind: "info"})
				return
			}
			title, doing, done := "Undo?", "Undoing…", "Undone"
			if redo {
				title, doing, done = "Redo?", "Redoing…", "Redone"
			}
			a.confirm(title, step.Describe(redo), title[:len(title)-1], func() {
				dirty := core.HasTrackedChanges(files)
				a.run(doing, func() error { return repo.ApplyUndo(step, redo, dirty) }, func() { a.toast(done, step.Describe(redo)) })
			})
		})
	})
}
