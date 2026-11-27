package app

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

// The panels of what lazygit has beside the files, the log, the branches
// and the stash: a remote's branches, the tags, the reflog, the worktrees
// and the submodules. Each is a list beside the detail of the item chosen,
// as Branches is.

// refRowHeight is the height of a row of these lists, as a branch's.
const refRowHeight = rowHeight + 4

// detailCard is the head of a detail pane: an icon and a title, lines of
// information under it, and a row of buttons.
func (a *App) detailCard(c *ui.Context, pal *palette, svg *ui.SVG, title string, info []string, buttons func()) {
	tt := c.Theme()
	indent := float32(28)
	if a.detailW > 0 && a.detailW < 420 {
		indent = 0
	}
	ui.Column(c).Padding(16, 18, 14, 18).Gap(10).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, svg).FontSize(18).TextColor(tt.Accent).Shrink(0)
				t := ui.Text(c, title).FontSize(16).FontWeight(700).Selectable().Shrink(1).MinWidth(0).Tooltip(title)
				if a.detailW > 0 && a.detailW < 560 {
					t.MaxLines(2)
				} else {
					t.SingleLine()
				}
			})
			for _, line := range info {
				if line != "" && line != title {
					ui.Text(c, line).FontSize(12).TextColor(pal.muted).SingleLine().Selectable().Padding(0, 0, 0, 28)
				}
			}
		})
		if buttons != nil {
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Padding(0, 0, 0, indent).Children(buttons)
		}
	})
}

// dangerButton is a header button for what loses work, in red while it
// can be done.
func dangerButton(c *ui.Context, label, tip string, enabled bool) ui.Element {
	b := headerButton(c, label).Disabled(!enabled).Tooltip(tip)
	if enabled {
		b.TextColor(t(c).Danger)
	}
	return b
}

// openRepository opens a repository at path in a window of its own, or
// brings its window to the front.
func (a *App) openRepository(path string) {
	if a.host.OpenRecent != nil {
		a.host.OpenRecent(path)
	}
}

// listView builds one of these lists: n rows of row, chosen by sel, with
// empty shown when there are none, and Enter or a double click running
// submit.
func (a *App) listView(c *ui.Context, pal *palette, key string, st *ui.ListState, sel *int, n int,
	id func(i int) any, label func(i int) string, row func(i int, sel bool), empty func(), submit func()) {
	st.Selected = sel
	st.Key = id
	st.Label = label
	hot := a.hot(c, paneList)
	var list ui.Element
	listIn(c, hot, func() {
		list = ui.List(c.Key(key), st, n, func(i int) {
			row(i, hot && i == *sel)
		}).Grow(1).Padding(4, 6).FocusBind(&a.pane, paneList).FocusRing(false).Children(func() {
			if n == 0 && empty != nil {
				empty()
			}
		})
	})
	if list.Submitted() && submit != nil && *sel >= 0 && *sel < n {
		submit()
	}
}

// Remote branches

// showRemote shows the branches of a remote.
func (a *App) showRemote(name string) {
	if name != a.remoteName {
		a.remoteName, a.remoteBranchSel = name, 0
		a.remoteBranchesList = ui.ListState{}
	}
	a.section, a.pane = secRemotes, paneList
}

// goToRemotes shows the branches of the remote last shown, or the first.
func (a *App) goToRemotes() {
	name := a.remoteName
	if r := a.remote(); r == nil && a.snap != nil && len(a.snap.Remotes) > 0 {
		name = a.snap.Remotes[0].Name
	}
	a.showRemote(name)
}

func (a *App) remote() *models.Remote {
	if a.snap == nil {
		return nil
	}
	for _, r := range a.snap.Remotes {
		if r.Name == a.remoteName {
			return r
		}
	}
	return nil
}

func (a *App) remoteBranches() []*models.RemoteBranch {
	if r := a.remote(); r != nil {
		return r.Branches
	}
	return nil
}

func (a *App) selectedRemoteBranch() *models.RemoteBranch {
	bs := a.remoteBranches()
	if a.remoteBranchSel < 0 || a.remoteBranchSel >= len(bs) {
		return nil
	}
	return bs[a.remoteBranchSel]
}

// trackedBy is the local branch tracking a remote branch, or nil.
func (a *App) trackedBy(rb *models.RemoteBranch) *models.Branch {
	for _, b := range a.snap.Branches {
		if b.UpstreamRemote == rb.RemoteName && b.UpstreamBranch == rb.Name {
			return b
		}
	}
	return nil
}

func (a *App) remoteBranchList(c *ui.Context, pal *palette) {
	r := a.remote()
	branches := a.remoteBranches()
	title := "No remote"
	if r != nil {
		title = plural(len(branches), "branch") + " on " + r.Name
	}
	paneHeader(c, pal, title, func() {
		if headerButton(c, "Fetch").Disabled(r == nil).Tooltip("Fetch " + a.remoteName + " (f)").Clicked() {
			a.fetchRemote()
		}
	})
	a.listView(c, pal, "remoteBranches", &a.remoteBranchesList, &a.remoteBranchSel, len(branches),
		func(i int) any { return branches[i].FullName() },
		func(i int) string { return branches[i].Name },
		func(i int, sel bool) {
			rb := branches[i]
			tt := c.Theme()
			ui.Row(c).Height(refRowHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBranch).FontSize(13).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				ui.Text(c, rb.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0).Tooltip(rb.FullName())
				ui.Spacer(c)
				if b := a.trackedBy(rb); b != nil {
					label := "tracked"
					if b.Head {
						label = "checked out"
					}
					ui.Text(c, label).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0).Tooltip("By " + b.Name)
				}
			}).ContextMenu(func(m *ui.Menu) { a.remoteBranchMenu(m, rb) })
		},
		func() {
			emptyState(c, pal, iconRemote, "No branches", "Fetch the remote to see its branches.")
		},
		func() { a.pane = paneFiles })
}

func (a *App) remoteBranchMenu(m *ui.Menu, rb *models.RemoteBranch) {
	if m.Item("Checkout").Shortcut(0, ui.KeySpace).Chosen() {
		a.checkoutRemoteBranch(rb)
	}
	if m.Item("New Branch From Here…").Shortcut(0, ui.KeyN).Chosen() {
		a.newBranchFrom(rb.FullName())
	}
	m.Separator()
	if m.Item("Merge Into Current Branch").Shortcut(ui.Shift, ui.KeyM).Chosen() {
		a.mergeRef(rb.FullName())
	}
	if m.Item("Rebase Current Branch Onto This").Shortcut(0, ui.KeyR).Chosen() {
		a.rebaseOntoRef(rb.FullName())
	}
	m.Separator()
	if m.Item("Delete on "+rb.RemoteName+"…").Shortcut(0, ui.KeyD).Chosen() {
		a.deleteRemoteBranch(rb)
	}
}

func (a *App) remoteBranchDetail(c *ui.Context, pal *palette) {
	rb := a.selectedRemoteBranch()
	if rb == nil {
		emptyState(c, pal, iconRemote, "No remote branch chosen", "")
		return
	}
	a.loadRefCommits("remote:"+rb.FullName(), func(r *core.Repo) ([]*models.Commit, [][]core.Pipe, error) {
		return r.RefCommits(rb.FullRefName(), branchCommitLimit)
	})
	info := "Not tracked by a local branch"
	if b := a.trackedBy(rb); b != nil {
		info = "Tracked by " + b.Name
		if b.Head {
			info += ", checked out"
		}
	}
	a.detailCard(c, pal, iconRemote, rb.FullName(), []string{info}, func() {
		label := "Checkout"
		if a.trackedBy(rb) == nil {
			label = "Checkout as Local Branch"
		}
		if defaultButton(c, label, true).Height(24).Padding(0, 10).FontSize(12).Tooltip("Check out (space)").Clicked() {
			a.checkoutRemoteBranch(rb)
		}
		if headerButton(c, "Merge").Tooltip("Merge " + rb.FullName() + " into " + a.headName() + " (M)").Clicked() {
			a.mergeRef(rb.FullName())
		}
		if headerButton(c, "Rebase").Tooltip("Rebase " + a.headName() + " onto " + rb.FullName() + " (r)").Clicked() {
			a.rebaseOntoRef(rb.FullName())
		}
		ui.Spacer(c)
		if dangerButton(c, "Delete…", "Delete the branch on "+rb.RemoteName+" (d)", true).Clicked() {
			a.deleteRemoteBranch(rb)
		}
	})
	a.refCommits(c, pal, rb.FullName())
}

func (a *App) checkoutRemoteBranch(rb *models.RemoteBranch) {
	a.run("Checking out "+rb.Name+"…", func(r *core.Repo) error { return r.CheckoutRemoteBranch(rb) },
		func() { a.toast("Checked out "+rb.Name, "") })
}

func (a *App) mergeRef(name string) {
	a.confirm("Merge "+name+" into "+a.headName()+"?", "", "Merge", func() {
		a.run("Merging…", func(r *core.Repo) error { return r.Merge(name) })
	})
}

func (a *App) rebaseOntoRef(name string) {
	a.confirm("Rebase "+a.headName()+" onto "+name+"?", "", "Rebase", func() {
		a.run("Rebasing…", func(r *core.Repo) error { return r.RebaseOnto(name) })
	})
}

func (a *App) deleteRemoteBranch(rb *models.RemoteBranch) {
	a.confirm("Delete "+rb.Name+" on "+rb.RemoteName+"?", "The branch is deleted on the remote, for everyone who uses it.", "Delete", func() {
		a.run("Deleting…", func(r *core.Repo) error { return r.DeleteRemoteBranch(rb) },
			func() { a.toast("Deleted "+rb.FullName(), "") })
	})
}

func (a *App) fetchRemote() {
	name := a.remoteName
	if name == "" {
		return
	}
	a.run("Fetching "+name+"…", func(r *core.Repo) error { return r.FetchRemote(name) }, func() { a.toast("Fetched "+name, "") })
}

// Tags

func (a *App) selectedTag() *models.Tag {
	if a.snap == nil || a.tagSel < 0 || a.tagSel >= len(a.snap.Tags) {
		return nil
	}
	return a.snap.Tags[a.tagSel]
}

func (a *App) tagList(c *ui.Context, pal *palette) {
	tags := a.snap.Tags
	paneHeader(c, pal, plural(len(tags), "tag"), func() {
		if headerButton(c, "New Tag…").Tooltip("Tag HEAD (n)").Clicked() {
			a.newTag("HEAD")
		}
	})
	a.listView(c, pal, "tags", &a.tagsList, &a.tagSel, len(tags),
		func(i int) any { return tags[i].Name },
		func(i int) string { return tags[i].Name },
		func(i int, sel bool) {
			tag := tags[i]
			tt := c.Theme()
			ui.Row(c).Height(refRowHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconTag).FontSize(13).TextColor(tint(sel, tt, pal.modified)).Shrink(0)
				ui.Text(c, tag.Name).FontSize(13).FontWeight(500).SingleLine().Shrink(0).MaxWidth(180).Tooltip(tag.Name)
				ui.Text(c, tag.Message).FontSize(12).TextColor(tint(sel, tt, pal.muted)).SingleLine().Shrink(1).MinWidth(0).Tooltip(tag.Message)
			}).ContextMenu(func(m *ui.Menu) { a.tagMenu(m, tag) })
		},
		func() { emptyState(c, pal, iconTag, "No tags", "Tag a commit to mark a release.") },
		func() { a.pane = paneFiles })
}

func (a *App) tagMenu(m *ui.Menu, tag *models.Tag) {
	if m.Item("Checkout").Shortcut(0, ui.KeySpace).Chosen() {
		a.checkoutTag(tag)
	}
	if m.Item("New Branch From Here…").Chosen() {
		a.newBranchFrom(tag.Name)
	}
	if m.Item("Push…").Shortcut(ui.Shift, ui.KeyP).Chosen() {
		a.pushTag(tag)
	}
	m.Separator()
	if m.Item("Delete…").Shortcut(0, ui.KeyD).Chosen() {
		a.deleteTag(tag)
	}
}

func (a *App) tagDetail(c *ui.Context, pal *palette) {
	tag := a.selectedTag()
	if tag == nil {
		emptyState(c, pal, iconTag, "No tag chosen", "")
		return
	}
	a.loadRefCommits("tag:"+tag.Name, func(r *core.Repo) ([]*models.Commit, [][]core.Pipe, error) {
		return r.RefCommits(tag.FullRefName(), branchCommitLimit)
	})
	a.detailCard(c, pal, iconTag, tag.Name, []string{tag.Message}, func() {
		if defaultButton(c, "Checkout", true).Height(24).Padding(0, 10).FontSize(12).Tooltip("Check out, detached (space)").Clicked() {
			a.checkoutTag(tag)
		}
		if headerButton(c, "New Branch…").Tooltip("A branch from " + tag.Name).Clicked() {
			a.newBranchFrom(tag.Name)
		}
		if headerButton(c, "Push…").Disabled(len(a.snap.Remotes) == 0).Tooltip("Push to a remote (P)").Clicked() {
			a.pushTag(tag)
		}
		ui.Spacer(c)
		if dangerButton(c, "Delete…", "Delete the tag (d)", true).Clicked() {
			a.deleteTag(tag)
		}
	})
	a.refCommits(c, pal, tag.Name)
}

// newTag asks for a tag's name, then its message, empty for a lightweight
// tag, and tags ref.
func (a *App) newTag(ref string) {
	a.prompt("New tag on "+ref, "", "Next", false, func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		d := a.prompt("Message for "+name, "", "Create Tag", true, func(msg string) {
			msg = strings.TrimSpace(msg)
			a.run("Tagging…", func(r *core.Repo) error { return r.CreateTag(name, ref, msg) }, func() {
				a.section = secTags
				a.pickAfterLoad(secTags, name)
				a.toast("Tagged "+ref+" "+name, "")
			})
		})
		if d != nil {
			d.message = "With a message, the tag is annotated; without, it is lightweight."
		}
	})
}

func (a *App) checkoutTag(tag *models.Tag) {
	a.confirm("Check out "+tag.Name+"?", "The head is detached at the tag: commits made there belong to no branch.", "Checkout", func() {
		a.run("Checking out "+tag.Name+"…", func(r *core.Repo) error { return r.CheckoutRef(tag.FullRefName()) },
			func() { a.toast("Checked out "+tag.Name, "") })
	})
}

func (a *App) pushTag(tag *models.Tag) {
	if len(a.snap.Remotes) == 0 {
		return
	}
	var names []string
	for _, r := range a.snap.Remotes {
		names = append(names, r.Name)
	}
	push := func(remote string) {
		a.run("Pushing "+tag.Name+"…", func(r *core.Repo) error { return r.PushTag(remote, tag.Name) },
			func() { a.toast("Pushed "+tag.Name+" to "+remote, "") })
	}
	if len(names) == 1 {
		a.confirm("Push "+tag.Name+" to "+names[0]+"?", "", "Push", func() { push(names[0]) })
		return
	}
	if d := a.prompt("Push "+tag.Name+" to which remote?", names[0], "Push", false, func(v string) { push(strings.TrimSpace(v)) }); d != nil {
		d.message = "The remotes: " + strings.Join(names, ", ") + "."
	}
}

func (a *App) deleteTag(tag *models.Tag) {
	a.confirm("Delete tag "+tag.Name+"?", "It is deleted here; remotes keep theirs.", "Delete", func() {
		a.run("Deleting…", func(r *core.Repo) error { return r.DeleteTag(tag.Name) })
	})
}

// Reflog

func (a *App) selectedReflog() *models.Commit {
	if a.snap == nil || a.reflogSel < 0 || a.reflogSel >= len(a.snap.Reflog) {
		return nil
	}
	return a.snap.Reflog[a.reflogSel]
}

// reflogParts splits a reflog entry's subject into its action and what it
// says: "checkout: moving from a to b" into "checkout" and the rest.
func reflogParts(subject string) (action, rest string) {
	if action, rest, ok := strings.Cut(subject, ": "); ok {
		return action, rest
	}
	return "", subject
}

func (a *App) reflogListView(c *ui.Context, pal *palette) {
	entries := a.snap.Reflog
	title := plural(len(entries), "entry")
	if len(entries) >= core.ReflogLimit {
		title = fmt.Sprintf("Latest %d entries", len(entries))
	}
	paneHeader(c, pal, title, nil)
	a.listView(c, pal, "reflog", &a.reflogList, &a.reflogSel, len(entries),
		// Entries can repeat a commit: the row is its place.
		func(i int) any { return fmt.Sprintf("%d:%s", len(entries)-i, entries[i].Hash()) },
		func(i int) string { return entries[i].Name },
		func(i int, sel bool) {
			e := entries[i]
			tt := c.Theme()
			action, rest := reflogParts(e.Name)
			ui.Row(c).Height(refRowHeight).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, e.ShortHash()).Font(codeFont).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				if action != "" {
					ui.Text(c, action).FontSize(11).FontWeight(600).Padding(1, 6).Radius(4).SingleLine().Shrink(0).MaxWidth(130).
						TextColor(tint(sel, tt, pal.renamed)).Background(pal.renamed.Alpha(0.12))
				}
				ui.Text(c, rest).FontSize(13).SingleLine().Shrink(1).MinWidth(0).Tooltip(e.Name)
				ui.Spacer(c)
				ui.Text(c, relTime(e.UnixTimestamp)).FontSize(11).FontFeatures("tnum").TextColor(tint(sel, tt, pal.muted)).Shrink(0)
			}).ContextMenu(func(m *ui.Menu) { a.reflogMenu(m, e) })
		},
		func() { emptyState(c, pal, iconReflog, "No reflog", "") },
		func() { a.pane = paneFiles })
}

func (a *App) reflogMenu(m *ui.Menu, e *models.Commit) {
	if m.Item("Checkout Commit").Shortcut(0, ui.KeySpace).Chosen() {
		a.checkoutCommit(e)
	}
	if m.Item("New Branch From Here…").Shortcut(0, ui.KeyN).Chosen() {
		a.newBranchFrom(e.Hash())
	}
	if m.Item(copyLabel(a.isCopied(e))).Shortcut(ui.Shift, ui.KeyC).Chosen() {
		a.toggleCopy("the reflog", a.snap.Reflog, a.reflogSel)
	}
	m.Separator()
	if m.Item("Reset Soft to Here").Chosen() {
		a.resetTo(e, core.ResetSoft)
	}
	if m.Item("Reset Mixed to Here").Chosen() {
		a.resetTo(e, core.ResetMixed)
	}
	if m.Item("Reset Hard to Here…").Shortcut(0, ui.KeyG).Chosen() {
		a.resetTo(e, core.ResetHard)
	}
}

func (a *App) reflogDetail(c *ui.Context, pal *palette) {
	e := a.selectedReflog()
	if e == nil {
		emptyState(c, pal, iconReflog, "No reflog entry chosen", "")
		return
	}
	repo := a.repo
	key := "commit:" + e.Hash()
	a.detailOf(key, func() ([]*models.CommitFile, string, error) {
		files, err := repo.CommitFiles(e)
		if err != nil {
			return nil, "", err
		}
		msg, _ := repo.CommitMessage(e)
		return files, msg, nil
	})
	subject, _, _ := strings.Cut(strings.TrimSpace(a.detailMsg), "\n")
	action, rest := reflogParts(e.Name)
	title := rest
	if action != "" {
		title = titleCase(action) + ": " + rest
	}
	info := []string{e.Hash()[:min(12, len(e.Hash()))] + " · " + ago(e.UnixTimestamp)}
	if subject != "" {
		info = append([]string{subject}, info...)
	}
	a.detailCard(c, pal, iconReflog, title, info, func() {
		if headerButton(c, "Checkout").Tooltip("Check out the commit, detached (space)").Clicked() {
			a.checkoutCommit(e)
		}
		if headerButton(c, "New Branch…").Tooltip("A branch from the commit (n)").Clicked() {
			a.newBranchFrom(e.Hash())
		}
		ui.Spacer(c)
		ui.MenuButton(c, "Reset to Here", func(m *ui.Menu) {
			if m.Item("Soft: keep the changes staged").Chosen() {
				a.resetTo(e, core.ResetSoft)
			}
			if m.Item("Mixed: keep the changes unstaged").Chosen() {
				a.resetTo(e, core.ResetMixed)
			}
			if m.Item("Hard: discard the changes…").Chosen() {
				a.resetTo(e, core.ResetHard)
			}
		}).Height(24).FontSize(12)
	})
	a.filesAndDiff(c, pal, key, func(f *models.CommitFile) (string, error) { return repo.CommitFileDiff(e, f) })
}

func (a *App) checkoutCommit(e *models.Commit) {
	hash := e.Hash()
	a.confirm("Check out "+e.ShortHash()+"?", "The head is detached at the commit: commits made there belong to no branch.", "Checkout", func() {
		a.run("Checking out…", func(r *core.Repo) error { return r.CheckoutRef(hash) })
	})
}

// resetTo resets the branch checked out to a reflog entry's commit,
// asking first for a hard reset, which loses the changes.
func (a *App) resetTo(e *models.Commit, mode core.ResetMode) {
	hash := e.Hash()
	reset := func() {
		a.run("Resetting…", func(r *core.Repo) error { return r.ResetTo(hash, mode) },
			func() { a.toast("Reset "+a.headName()+" to "+e.ShortHash(), "") })
	}
	if mode != core.ResetHard {
		reset()
		return
	}
	a.confirm("Reset "+a.headName()+" hard to "+e.ShortHash()+"?",
		"Changes to tracked files are lost. Commits left behind stay in the reflog.", "Reset Hard", reset)
}

// Worktrees

func (a *App) selectedWorktree() *models.Worktree {
	if a.snap == nil || a.worktreeSel < 0 || a.worktreeSel >= len(a.snap.Worktrees) {
		return nil
	}
	return a.snap.Worktrees[a.worktreeSel]
}

// worktreeBranch is what a worktree has checked out: its branch, or its
// head's commit, detached.
func worktreeBranch(w *models.Worktree) string {
	if w.Branch != "" {
		return w.Branch
	}
	if w.Head != "" {
		return w.Head[:min(8, len(w.Head))] + " (detached)"
	}
	return ""
}

func (a *App) worktreeList(c *ui.Context, pal *palette) {
	worktrees := a.snap.Worktrees
	paneHeader(c, pal, plural(len(worktrees), "worktree"), func() {
		if headerButton(c, "New Worktree…").Tooltip("New worktree (n)").Clicked() {
			a.newWorktree()
		}
	})
	a.listView(c, pal, "worktrees", &a.worktreesList, &a.worktreeSel, len(worktrees),
		func(i int) any { return worktrees[i].Path },
		func(i int) string { return worktrees[i].Name },
		func(i int, sel bool) {
			w := worktrees[i]
			tt := c.Theme()
			ui.Row(c).Height(refRowHeight+14).Padding(0, 12).Gap(10).AlignItems(ui.Center).Children(func() {
				color := pal.muted
				if w.IsCurrent {
					color = tt.Accent
				}
				ui.Icon(c, iconWorktree).FontSize(14).TextColor(tint(sel, tt, color)).Shrink(0)
				ui.Column(c).Gap(1).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						n := ui.Text(c, w.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
						if w.IsCurrent {
							n.FontWeight(600)
						}
						if b := worktreeBranch(w); b != "" {
							ui.Text(c, b).FontSize(11).FontWeight(600).Padding(1, 6).Radius(4).SingleLine().Shrink(1).MinWidth(0).MaxWidth(160).
								TextColor(tint(sel, tt, pal.renamed)).Background(pal.renamed.Alpha(0.12))
						}
					})
					ui.Text(c, abbreviateHome(w.Path)).FontSize(11).TextColor(tint(sel, tt, pal.muted)).SingleLine().Tooltip(w.Path)
				})
				switch {
				case w.IsPathMissing:
					ui.Text(c, "missing").FontSize(11).TextColor(tint(sel, tt, pal.deleted)).Shrink(0)
				case w.IsCurrent:
					ui.Text(c, "this window").FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				case w.IsMain:
					ui.Text(c, "main").FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				}
			}).ContextMenu(func(m *ui.Menu) {
				if m.Item("Open in a Window").Shortcut(0, ui.KeySpace).Disabled(w.IsCurrent || w.IsPathMissing).Chosen() {
					a.openRepository(w.Path)
				}
				if m.Item("New Worktree…").Shortcut(0, ui.KeyN).Chosen() {
					a.newWorktree()
				}
				m.Separator()
				if m.Item("Remove…").Shortcut(0, ui.KeyD).Disabled(w.IsMain || w.IsCurrent).Chosen() {
					a.removeWorktree(w)
				}
			})
		},
		nil,
		func() {
			if w := a.selectedWorktree(); w != nil && !w.IsCurrent && !w.IsPathMissing {
				a.openRepository(w.Path)
			}
		})
}

func (a *App) worktreeDetail(c *ui.Context, pal *palette) {
	w := a.selectedWorktree()
	if w == nil {
		emptyState(c, pal, iconWorktree, "No worktree chosen", "")
		return
	}
	var info []string
	if b := worktreeBranch(w); b != "" {
		info = append(info, "On "+b)
	}
	info = append(info, w.Path)
	switch {
	case w.IsPathMissing:
		info = append(info, "Its folder is gone: remove it to tidy up")
	case w.IsMain && w.IsCurrent:
		info = append(info, "The main worktree, open in this window")
	case w.IsMain:
		info = append(info, "The main worktree")
	case w.IsCurrent:
		info = append(info, "Open in this window")
	}
	a.detailCard(c, pal, iconWorktree, w.Name, info, func() {
		if defaultButton(c, "Open in a Window", !w.IsCurrent && !w.IsPathMissing).Height(24).Padding(0, 10).FontSize(12).
			Tooltip("Open the worktree in a window of its own (space)").Clicked() {
			a.openRepository(w.Path)
		}
		ui.Spacer(c)
		if dangerButton(c, "Remove…", "Remove the worktree (d)", !w.IsMain && !w.IsCurrent).Clicked() {
			a.removeWorktree(w)
		}
	})
	emptyState(c, pal, iconWorktree, "", "A worktree is another folder of the repository, with a branch of its own checked out: work on two branches at once.")
}

// newWorktree asks for a new worktree's folder and branch, from the
// branch chosen in Branches, or HEAD.
func (a *App) newWorktree() {
	base := a.headName()
	if b := a.selectedBranch(); b != nil && a.section == secBranches {
		base = b.Name
	}
	repoName := a.repo.Name()
	a.prompt("New worktree from "+base, "../"+repoName+"-", "Next", false, func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		d := a.prompt("Branch for the worktree", filepath.Base(path), "Create Worktree", false, func(branch string) {
			branch = strings.TrimSpace(branch)
			a.run("Making the worktree…", func(r *core.Repo) error { return r.AddWorktree(path, base, branch) }, func() {
				a.section = secWorktrees
				a.pickAfterLoad(secWorktrees, a.repo.WorktreePath(path))
				a.toast("Made the worktree", a.repo.WorktreePath(path))
			})
		})
		if d != nil {
			d.message = "A new branch from " + base + ". Leave it empty to check out " + base + " itself, which no other worktree may have checked out."
		}
	})
}

// removeWorktree removes a worktree, asking again for one with changes,
// which are lost with it.
func (a *App) removeWorktree(w *models.Worktree) {
	a.confirm("Remove the worktree "+w.Name+"?", "Its folder, "+w.Path+", is deleted. Its branch stays.", "Remove", func() {
		dirty := false
		a.run("Removing the worktree…", func(r *core.Repo) error {
			err := r.RemoveWorktree(w, false)
			if errors.Is(err, core.ErrWorktreeDirty) {
				dirty = true
				return nil
			}
			return err
		}, func() {
			if !dirty {
				return
			}
			a.confirm(w.Name+" has changes", "Its changes and untracked files are lost with it.", "Remove Anyway", func() {
				a.run("Removing the worktree…", func(r *core.Repo) error { return r.RemoveWorktree(w, true) })
			})
		})
	})
}

// Submodules

func (a *App) selectedSubmodule() *core.Submodule {
	if a.snap == nil || a.submoduleSel < 0 || a.submoduleSel >= len(a.snap.Submodules) {
		return nil
	}
	return a.snap.Submodules[a.submoduleSel]
}

// submoduleState is what a submodule's status says, and its color.
func submoduleState(pal *palette, s *core.Submodule) (string, ui.Color) {
	switch s.Status {
	case '-':
		return "not initialized", pal.muted
	case '+':
		return "other commit", pal.modified
	case 'U':
		return "conflicts", pal.deleted
	}
	return "", pal.muted
}

func (a *App) submoduleList(c *ui.Context, pal *palette) {
	subs := a.snap.Submodules
	paneHeader(c, pal, plural(len(subs), "submodule"), func() {
		if headerButton(c, "Update All").Disabled(len(subs) == 0).Tooltip("Initialize and update every submodule (U)").Clicked() {
			a.updateAllSubmodules()
		}
		if headerButton(c, "Add…").Tooltip("Add a submodule (n)").Clicked() {
			a.addSubmodule()
		}
	})
	a.listView(c, pal, "submodules", &a.submodulesList, &a.submoduleSel, len(subs),
		func(i int) any { return subs[i].Path },
		func(i int) string { return subs[i].Name },
		func(i int, sel bool) {
			s := subs[i]
			tt := c.Theme()
			ui.Row(c).Height(refRowHeight+14).Padding(0, 12).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconSubmodule).FontSize(14).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				ui.Column(c).Gap(1).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, s.Name).FontSize(13).SingleLine()
					// Under its name, its path, or, the same, where it comes from.
					sub := s.Path
					if sub == s.Name {
						sub = abbreviateHome(s.Url)
					}
					ui.Text(c, sub).FontSize(11).TextColor(tint(sel, tt, pal.muted)).SingleLine().Tooltip(s.Url)
				})
				if state, color := submoduleState(pal, s); state != "" {
					ui.Text(c, state).FontSize(11).TextColor(tint(sel, tt, color)).Shrink(0)
				}
			}).ContextMenu(func(m *ui.Menu) {
				if m.Item("Open in a Window").Shortcut(0, ui.KeySpace).Disabled(!s.Initialized()).Chosen() {
					a.openRepository(filepath.Join(a.repo.Path, s.Path))
				}
				if m.Item("Update").Shortcut(0, ui.KeyU).Chosen() {
					a.updateSubmodule(s)
				}
				m.Separator()
				if m.Item("Remove…").Shortcut(0, ui.KeyD).Chosen() {
					a.removeSubmodule(s)
				}
			})
		},
		func() {
			emptyState(c, pal, iconSubmodule, "No submodules", "A submodule is another repository, checked out in a folder of this one.")
		},
		func() {
			if s := a.selectedSubmodule(); s != nil && s.Initialized() {
				a.openRepository(filepath.Join(a.repo.Path, s.Path))
			}
		})
}

func (a *App) submoduleDetail(c *ui.Context, pal *palette) {
	s := a.selectedSubmodule()
	if s == nil {
		emptyState(c, pal, iconSubmodule, "No submodule chosen", "")
		return
	}
	state, _ := submoduleState(pal, s)
	head := ""
	if s.Head != "" {
		head = "At " + s.Head[:min(12, len(s.Head))]
		if state != "" {
			head += " · " + state
		}
	} else if state != "" {
		head = titleCase(state)
	}
	a.detailCard(c, pal, iconSubmodule, s.Name, []string{s.Path, s.Url, head}, func() {
		if defaultButton(c, "Open in a Window", s.Initialized()).Height(24).Padding(0, 10).FontSize(12).
			Tooltip("Open the submodule in a window of its own (space)").Clicked() {
			a.openRepository(filepath.Join(a.repo.Path, s.Path))
		}
		label := "Update"
		if !s.Initialized() {
			label = "Initialize"
		}
		if headerButton(c, label).Tooltip("Check out the commit recorded for it (u)").Clicked() {
			a.updateSubmodule(s)
		}
		ui.Spacer(c)
		if dangerButton(c, "Remove…", "Remove the submodule (d)", true).Clicked() {
			a.removeSubmodule(s)
		}
	})
	emptyState(c, pal, iconSubmodule, "", "Open it in a window of its own to see its changes, history and branches.")
}

func (a *App) updateSubmodule(s *core.Submodule) {
	a.run("Updating "+s.Name+"…", func(r *core.Repo) error { return r.UpdateSubmodule(s) }, func() { a.toast("Updated "+s.Name, "") })
}

func (a *App) updateAllSubmodules() {
	a.run("Updating submodules…", (*core.Repo).UpdateAllSubmodules, func() { a.toast("Updated the submodules", "") })
}

// addSubmodule asks for a repository's URL, then the folder it goes in.
func (a *App) addSubmodule() {
	a.prompt("Add a submodule", "", "Next", false, func(url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		def := strings.TrimSuffix(filepath.Base(strings.TrimRight(url, "/")), ".git")
		a.prompt("Folder for "+def, def, "Add Submodule", false, func(path string) {
			a.run("Adding the submodule…", func(r *core.Repo) error { return r.AddSubmodule(url, path) }, func() {
				a.section = secSubmodules
				a.pickAfterLoad(secSubmodules, filepath.Clean(strings.TrimSpace(path)))
				a.toast("Added the submodule", "Commit it to record it in the repository.")
			})
		})
	})
}

func (a *App) removeSubmodule(s *core.Submodule) {
	a.confirm("Remove the submodule "+s.Name+"?", "Its folder, "+s.Path+", and its entry in .gitmodules are removed, as a change to commit.", "Remove", func() {
		a.run("Removing "+s.Name+"…", func(r *core.Repo) error { return r.RemoveSubmodule(s) })
	})
}
