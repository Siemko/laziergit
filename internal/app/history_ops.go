package app

import (
	"path/filepath"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
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

// Copies are the commits copied to cherry-pick, which every window of a
// repository and of its worktrees shares: their commits are the same
// objects, so a window pastes what another copied.
type Copies struct {
	commits []copiedCommit
	// source names the log they came from, copying from another starting
	// afresh, and label says it; worktree is the work tree of the window
	// that copied them.
	source, label, worktree string
}

// copySource is a log commits are copied from: its name for the copies,
// what the banner calls it, and the worktree of the window copying.
type copySource struct{ id, label, worktree string }

func (cp *Copies) clear() { *cp = Copies{} }

func (cp *Copies) has(hash string) bool {
	return slices.ContainsFunc(cp.commits, func(cc copiedCommit) bool { return cc.commit.Hash() == hash })
}

// remove takes a commit out of the copies, whatever log it was copied
// from, and reports whether it was one. The last taken out, the copies
// say nothing more of where they came from.
func (cp *Copies) remove(hash string) bool {
	i := slices.IndexFunc(cp.commits, func(cc copiedCommit) bool { return cc.commit.Hash() == hash })
	if i < 0 {
		return false
	}
	cp.commits = slices.Delete(cp.commits, i, i+1)
	if len(cp.commits) == 0 {
		cp.clear()
	}
	return true
}

// add copies rows of commits, the log of src as it is now. The copies
// start afresh for another log; those of this one take their places in it
// now (reindex), which order the paste: places from two versions of a log
// wouldn't.
func (cp *Copies) add(src copySource, commits []*models.Commit, rows []int) {
	if src.id != cp.source {
		*cp = Copies{source: src.id, label: src.label, worktree: src.worktree}
	}
	cp.reindex(commits)
	for _, i := range rows {
		if i >= 0 && i < len(commits) && !cp.has(commits[i].Hash()) {
			cp.commits = append(cp.commits, copiedCommit{commits[i], i})
		}
	}
}

// reindex gives the copies their places in commits: the row a copy was
// at, while it still holds it, as a reflog repeats commits; else the
// first that does. Copies not in it, as past the commits a branch's log
// loads, or rewritten since, go after them, older than all, in the order
// they had.
func (cp *Copies) reindex(commits []*models.Commit) {
	at := make(map[string]int, len(commits))
	for i := len(commits) - 1; i >= 0; i-- {
		at[commits[i].Hash()] = i
	}
	slices.SortStableFunc(cp.commits, func(x, y copiedCommit) int { return x.index - y.index })
	past := len(commits)
	for j := range cp.commits {
		cc := &cp.commits[j]
		hash := cc.commit.Hash()
		switch i, ok := at[hash]; {
		case cc.index >= 0 && cc.index < len(commits) && commits[cc.index].Hash() == hash:
		case ok:
			cc.index = i
		default:
			cc.index = past
			past++
		}
	}
}

// copies are the window's repository's copies: those of every window of
// it, or the window's own without a host that shares them, as in tests.
func (a *App) copies() *Copies {
	if a.repo != nil && a.host.Copies != nil {
		return a.host.Copies(a.repo.CommonDir())
	}
	return &a.ownCopies
}

// copiesChanged has the other windows show the copies as they are now.
func (a *App) copiesChanged() {
	if a.host.CopiesChanged != nil {
		a.host.CopiesChanged()
	}
}

func (a *App) hasCopies() bool { return len(a.copies().commits) > 0 }

func (a *App) clearCopies() {
	a.copies().clear()
	a.copiesChanged()
}

func (a *App) isCopied(c *models.Commit) bool { return a.copies().has(c.Hash()) }

// copySource is the log named source, in this window: the log of HEAD,
// or the reflog, is the worktree's own; a branch's or a tag's is the same
// in every worktree.
func (a *App) copySource(source string) copySource {
	src := copySource{id: source, label: source, worktree: a.repo.Path}
	switch source {
	case "HEAD":
		src.label = a.headName()
		src.id += "@" + a.repo.Path
	case "the reflog":
		src.id += "@" + a.repo.Path
	}
	return src
}

// toggleCopy copies commit i of a log to cherry-pick, or takes it out of
// the copies, from whichever log it was copied. Copying from another log
// starts the copies afresh.
func (a *App) toggleCopy(source string, commits []*models.Commit, i int) {
	if i < 0 || i >= len(commits) || a.repo == nil {
		return
	}
	cp := a.copies()
	defer a.copiesChanged()
	if !cp.remove(commits[i].Hash()) {
		cp.add(a.copySource(source), commits, []int{i})
	}
}

// pasteCopied cherry-picks the copies onto HEAD, after asking. Once done,
// it takes those it pasted out of the copies, and leaves those copied
// meanwhile, as in another window.
func (a *App) pasteCopied() {
	cp := a.copies()
	if len(cp.commits) == 0 {
		return
	}
	copies := slices.Clone(cp.commits)
	slices.SortFunc(copies, func(x, y copiedCommit) int { return x.index - y.index })
	commits := make([]*models.Commit, len(copies))
	for i, cc := range copies {
		commits[i] = cc.commit
	}
	a.confirm("Cherry-pick "+plural(len(commits), "commit")+" onto "+a.headName()+"?",
		"They are applied oldest first, as new commits.", "Cherry-Pick", func() {
			pasted := func() {
				for _, c := range commits {
					cp.remove(c.Hash())
				}
				a.copiesChanged()
			}
			a.run("Cherry-picking…", func(r *core.Repo) error {
				err := r.CherryPick(commits)
				if err == nil && a.host.Main != nil {
					// The copies are the repository's other windows' too,
					// which this one closing mustn't leave offering them.
					a.host.Main(pasted)
				}
				return err
			}, func() {
				if a.host.Main == nil {
					pasted()
				}
				a.toast("Cherry-picked "+plural(len(commits), "commit"), "")
			})
		})
}

// copiedFrom says where the copies came from, for the window: nothing for
// its own branch's log, a branch's name, and another worktree's name.
func (a *App) copiedFrom() string {
	cp := a.copies()
	other := a.repo != nil && cp.worktree != "" && cp.worktree != a.repo.Path
	from := ""
	if cp.source != "HEAD@"+cp.worktree || other {
		from = " from " + cp.label
	}
	if other {
		from += " in " + filepath.Base(cp.worktree)
	}
	return from
}

// copiedBanner shows the commits copied, and pastes them.
func (a *App) copiedBanner(c *ui.Context, pal *palette) {
	cp := a.copies()
	if len(cp.commits) == 0 {
		return
	}
	ui.Row(c.Key("copiedBanner")).Padding(7, 14).Gap(10).AlignItems(ui.Center).Shrink(0).Background(a.accent.Alpha(0.08)).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Icon(c, iconCopy).FontSize(14).TextColor(a.accent)
		ui.Text(c, plural(len(cp.commits), "commit")+" copied"+a.copiedFrom()).FontWeight(600).SingleLine().Shrink(1).MinWidth(0).
			Tooltip(cp.worktree)
		ui.Spacer(c)
		if ui.Button(c, "Clear").Clicked() {
			a.clearCopies()
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
	if a.repo == nil || a.snap == nil {
		return
	}
	repo := a.repo
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
				a.run(doing, func(r *core.Repo) error { return r.ApplyUndo(step, redo) }, func() { a.toast(done, step.Describe(redo)) })
			})
		})
	})
}
