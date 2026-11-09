package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

func (a *App) selectedBranch() *models.Branch {
	if a.snap == nil || a.branchSel < 0 || a.branchSel >= len(a.snap.Branches) {
		return nil
	}
	return a.snap.Branches[a.branchSel]
}

func (a *App) branchList(c *ui.Context, pal *palette) {
	branches := a.snap.Branches
	paneHeader(c, pal, plural(len(branches), "local branch"), func() {
		if headerButton(c, "New Branch…").Tooltip("New branch (n)").Clicked() {
			a.newBranch()
		}
	})
	a.branchesList.Selected = &a.branchSel
	a.branchesList.Key = func(i int) any { return branches[i].Name }
	a.branchesList.Label = func(i int) string { return branches[i].Name }
	hot := a.hot(c, paneList)
	var list ui.Element
	listIn(c, hot, func() {
		list = ui.List(c.Key("branches"), &a.branchesList, len(branches), func(i int) {
			a.branchRow(c, pal, branches[i], hot && i == a.branchSel)
		}).Grow(1).Padding(4, 6).FocusBind(&a.pane, paneList).FocusRing(false)
	})
	if list.Submitted() {
		a.pane = paneFiles // into the branch's commits, as lazygit's Enter
	}
}

func (a *App) branchRow(c *ui.Context, pal *palette, b *models.Branch, sel bool) {
	tt := c.Theme()
	ui.Row(c).Height(rowHeight+4).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
		if b.Head {
			ui.Icon(c, iconHead).FontSize(13).TextColor(tint(sel, tt, tt.Accent)).Tooltip("Checked out")
		} else {
			ui.Icon(c, iconBranch).FontSize(13).TextColor(tint(sel, tt, pal.muted))
		}
		name := b.Name
		if b.DetachedHead {
			name = b.DisplayName
		}
		n := ui.Text(c, name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
		if b.Head {
			n.FontWeight(600)
		}
		ui.Spacer(c)
		if b.UpstreamGone {
			ui.Text(c, "upstream gone").FontSize(11).TextColor(tint(sel, tt, pal.deleted))
		} else if b.IsTrackingRemote() {
			if ahead := b.AheadForPull; ahead != "" && ahead != "0" {
				ui.Text(c, "↑"+ahead).FontSize(11).FontWeight(600).FontFeatures("tnum").TextColor(tint(sel, tt, tt.Text.Alpha(0.7)))
			}
			if behind := b.BehindForPull; behind != "" && behind != "0" {
				ui.Text(c, "↓"+behind).FontSize(11).FontWeight(600).FontFeatures("tnum").TextColor(tint(sel, tt, tt.Text.Alpha(0.7)))
			}
		}
		ui.Text(c, recency(b)).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Width(28).TextAlign(ui.End)
	}).ContextMenu(func(m *ui.Menu) {
		if m.Item("Checkout").Shortcut(0, ui.KeySpace).Disabled(b.Head).Chosen() {
			a.checkout(b)
		}
		if m.Item("New Branch From Here…").Shortcut(0, ui.KeyN).Chosen() {
			a.newBranchFrom(b.Name)
		}
		m.Separator()
		if m.Item("Merge Into Current Branch").Shortcut(ui.Shift, ui.KeyM).Disabled(b.Head).Chosen() {
			a.merge(b)
		}
		if m.Item("Rebase Current Branch Onto This").Shortcut(0, ui.KeyR).Disabled(b.Head).Chosen() {
			a.rebaseOnto(b)
		}
		m.Separator()
		if m.Item("Delete…").Shortcut(0, ui.KeyD).Disabled(b.Head).Chosen() {
			a.deleteBranch(b)
		}
	})
}

// branchCommitLimit is how many of a branch's commits its detail shows.
const branchCommitLimit = 200

func (a *App) branchDetail(c *ui.Context, pal *palette) {
	b := a.selectedBranch()
	if b == nil {
		emptyState(c, pal, iconBranch, "No branches", "")
		return
	}
	tt := c.Theme()
	key := fmt.Sprintf("%d:%s", a.gen, b.Name)
	if key != a.branchKey {
		if a.branchName != b.Name {
			a.branchCommits, a.branchGraph, a.branchCommitSel = nil, nil, 0
			a.branchCommitsList = ui.ListState{}
		}
		a.branchKey, a.branchName = key, b.Name
		repo := a.repo
		a.host.Go(func() {
			commits, graph, err := repo.BranchCommits(b, branchCommitLimit)
			a.host.Post(func() {
				if a.branchKey != key {
					return
				}
				if err != nil {
					a.toastErr(err)
					return
				}
				a.branchCommits, a.branchGraph = commits, graph
				a.branchCommitSel = max(0, min(a.branchCommitSel, len(commits)-1))
			})
		})
	}
	// Narrow, the buttons start at the edge, to fit on one line.
	indent := float32(28)
	if a.detailW > 0 && a.detailW < 420 {
		indent = 0
	}
	ui.Column(c).Padding(16, 18, 14, 18).Gap(10).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Column(c).Gap(3).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBranch).FontSize(18).TextColor(tt.Accent).Shrink(0)
				title := ui.Text(c, b.Name).FontSize(16).FontWeight(700).Selectable().Shrink(1).MinWidth(0).Tooltip(b.Name)
				if a.detailW > 0 && a.detailW < 560 {
					title.MaxLines(2)
				} else {
					title.SingleLine()
				}
				if b.Head {
					ui.Text(c, "HEAD").FontSize(11).FontWeight(700).TextColor(tt.AccentText).Padding(2, 6).Radius(4).Background(tt.Accent).Shrink(0)
				}
			})
			info := "Not published"
			if b.IsTrackingRemote() {
				info = fmt.Sprintf("Tracks %s/%s · ↑%s ↓%s", b.UpstreamRemote, b.UpstreamBranch, orZero(b.AheadForPull), orZero(b.BehindForPull))
			}
			if r := recency(b); r != "" {
				if r == "now" {
					info += " · checked out just now"
				} else {
					info += " · checked out " + r + " ago"
				}
			}
			ui.Text(c, info).FontSize(12).TextColor(pal.muted).SingleLine().Padding(0, 0, 0, 28)
		})
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Padding(0, 0, 0, indent).Children(func() {
			if defaultButton(c, "Checkout", !b.Head).Height(24).Padding(0, 10).FontSize(12).Clicked() {
				a.checkout(b)
			}
			if headerButton(c, "Merge").Disabled(b.Head).Tooltip("Merge " + b.Name + " into " + a.headName() + " (M)").Clicked() {
				a.merge(b)
			}
			if headerButton(c, "Rebase").Disabled(b.Head).Tooltip("Rebase " + a.headName() + " onto " + b.Name + " (r)").Clicked() {
				a.rebaseOnto(b)
			}
			ui.Spacer(c)
			del := headerButton(c, "Delete…").Disabled(b.Head).Tooltip("Delete the branch (d)")
			if !b.Head {
				del.TextColor(t(c).Danger)
			}
			if del.Clicked() {
				a.deleteBranch(b)
			}
		})
	})
	commits, graph := a.branchCommits, a.branchGraph
	paneHeader(c, pal, "Commits on "+b.Name, func() {
		if len(commits) > 0 && a.detailW >= 560 {
			ui.Text(c, "⇧C copies to cherry-pick").FontSize(11).TextColor(pal.muted).SingleLine().Shrink(0)
		}
	})
	gw := graphWidth(graph)
	a.branchCommitsList.Selected = &a.branchCommitSel
	a.branchCommitsList.Key = func(i int) any { return commits[i].Hash() }
	a.branchCommitsList.Label = func(i int) string { return commits[i].Name }
	hot := a.hot(c, paneFiles)
	listIn(c, hot, func() {
		ui.List(c.Key("branchCommits"), &a.branchCommitsList, len(commits), func(i int) {
			a.commitRow(c, pal, commits[i], graph[i], gw, i == a.branchCommitSel, hot, a.detailW).ContextMenu(func(m *ui.Menu) {
				a.branchCommitSel = i
				if m.Item(copyLabel(a.isCopied(commits[i]))).Shortcut(ui.Shift, ui.KeyC).Chosen() {
					a.toggleCopy(b.Name, commits, i)
				}
				if len(a.copied) > 0 && m.Item("Paste Copied Commits onto "+a.headName()+"…").Shortcut(ui.Shift, ui.KeyV).Chosen() {
					a.pasteCopied()
				}
			})
		}).Grow(1).Padding(4, 6).FocusBind(&a.pane, paneFiles).FocusRing(false).Label("Branch commits")
	})
}

// recency is how long ago a branch was checked out; lazygit's "*" for the
// one checked out says nothing more than its dot.
func recency(b *models.Branch) string {
	if b.Head || b.Recency == "*" {
		return ""
	}
	return fresh(b.Recency)
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

func (a *App) checkout(b *models.Branch) {
	if b.Head {
		return
	}
	a.run("Checking out "+b.Name+"…", func(r *core.Repo) error { return r.Checkout(b.Name) }, func() { a.toast("Checked out "+b.Name, "") })
}

func (a *App) newBranch() {
	base := "HEAD"
	if b := a.selectedBranch(); b != nil {
		base = b.Name
	}
	a.newBranchFrom(base)
}

func (a *App) newBranchFrom(base string) {
	a.prompt("New branch from "+base, "", "Create", false, func(name string) {
		a.run("Creating "+name+"…", func(r *core.Repo) error { return r.NewBranch(name, base) }, func() { a.branchSel = 0 })
	})
}

func (a *App) merge(b *models.Branch) {
	a.confirm("Merge "+b.Name+" into "+a.headName()+"?", "", "Merge", func() {
		a.run("Merging…", func(r *core.Repo) error { return r.Merge(b.Name) })
	})
}

func (a *App) rebaseOnto(b *models.Branch) {
	a.confirm("Rebase "+a.headName()+" onto "+b.Name+"?", "", "Rebase", func() {
		a.run("Rebasing…", func(r *core.Repo) error { return r.RebaseOnto(b.Name) })
	})
}

// deleteBranch deletes a branch merged, and asks again before deleting
// one whose commits no other branch has.
func (a *App) deleteBranch(b *models.Branch) {
	a.confirm("Delete branch "+b.Name+"?", "", "Delete", func() {
		unmerged := false
		a.run("Deleting…", func(r *core.Repo) error {
			err := r.DeleteBranch(b.Name, false)
			if errors.Is(err, core.ErrNotMerged) {
				unmerged = true
				return nil
			}
			return err
		}, func() {
			if !unmerged {
				return
			}
			a.confirm(b.Name+" isn't fully merged", "Its commits that no other branch has are lost when it is deleted.", "Delete Anyway", func() {
				a.run("Deleting…", func(r *core.Repo) error { return r.DeleteBranch(b.Name, true) })
			})
		})
	})
}

func (a *App) headName() string {
	if a.snap != nil && a.snap.Head != nil {
		return a.snap.Head.Name
	}
	return "HEAD"
}

// Stash

func (a *App) selectedStash() *models.StashEntry {
	if a.snap == nil || a.stashSel < 0 || a.stashSel >= len(a.snap.Stash) {
		return nil
	}
	return a.snap.Stash[a.stashSel]
}

func (a *App) stashListView(c *ui.Context, pal *palette) {
	entries := a.snap.Stash
	paneHeader(c, pal, plural(len(entries), "stash"), func() {
		if headerButton(c, "Stash Changes").Disabled(len(a.snap.Files) == 0).Tooltip("Stash all changes (s)").Clicked() {
			a.stashAll()
		}
	})
	a.stashList.Selected = &a.stashSel
	a.stashList.Key = func(i int) any { return entries[i].Hash }
	a.stashList.Label = func(i int) string { return entries[i].Name }
	hot := a.hot(c, paneList)
	listIn(c, hot, func() {
		ui.List(c.Key("stash"), &a.stashList, len(entries), func(i int) {
			e := entries[i]
			sel := hot && i == a.stashSel
			tt := c.Theme()
			ui.Row(c).Height(rowHeight+4).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				branch, msg := stashParts(e.Name)
				ui.Text(c, e.RefName()).Font(codeFont).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
				ui.Text(c, msg).FontSize(13).SingleLine().Shrink(1).MinWidth(0).Tooltip(e.Name)
				ui.Spacer(c)
				if branch != "" {
					ui.Text(c, branch).FontSize(11).TextColor(tint(sel, tt, pal.muted)).SingleLine().Shrink(0).MaxWidth(140)
				}
				ui.Text(c, fresh(e.Recency)).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Shrink(0)
			}).ContextMenu(func(m *ui.Menu) {
				if m.Item("Pop").Shortcut(0, ui.KeyG).Chosen() {
					a.stashPop(e)
				}
				if m.Item("Apply").Shortcut(0, ui.KeySpace).Chosen() {
					a.run("Applying…", func(r *core.Repo) error { return r.StashApply(e) })
				}
				m.Separator()
				if m.Item("Drop…").Shortcut(0, ui.KeyD).Chosen() {
					a.stashDrop(e)
				}
			})
		}).Grow(1).Padding(4, 6).FocusBind(&a.pane, paneList).FocusRing(false).Children(func() {
			if len(entries) == 0 {
				emptyState(c, pal, iconStash, "No stashes", "Stash changes to set them aside.")
			}
		})
	})
}

func (a *App) stashDetail(c *ui.Context, pal *palette) {
	e := a.selectedStash()
	if e == nil {
		emptyState(c, pal, iconStash, "No stash chosen", "")
		return
	}
	repo := a.repo
	key := "stash:" + e.Hash
	a.detailOf(key, func() ([]*models.CommitFile, string, error) {
		files, err := repo.StashFiles(e)
		return files, "", err
	})
	// Narrow, the buttons go below the message rather than squeeze it.
	stacked := a.detailW > 0 && a.detailW < 440
	head := ui.Row(c)
	if stacked {
		head = ui.Column(c)
	}
	head.Padding(14, 16).Gap(12).AlignItems(ui.Center).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		branch, msg := stashParts(e.Name)
		text := ui.Column(c).Gap(3).Grow(1).Shrink(1).MinWidth(0)
		if stacked {
			text.AlignSelf(ui.Stretch)
		}
		text.Children(func() {
			ui.Text(c, msg).FontSize(14).FontWeight(600).MaxLines(3).Selectable()
			info := e.RefName()
			if branch != "" {
				info += " · on " + branch
			}
			if r := fresh(e.Recency); r == "now" {
				info += " · just now"
			} else if r != "" {
				info += " · " + r + " ago"
			}
			ui.Text(c, info).FontSize(12).TextColor(pal.muted).SingleLine()
		})
		buttons := ui.Row(c).Gap(8).Shrink(0)
		if stacked {
			buttons.AlignSelf(ui.End)
		}
		buttons.Children(func() {
			if ui.Button(c, "Apply").Clicked() {
				a.run("Applying…", func(r *core.Repo) error { return r.StashApply(e) })
			}
			if ui.PrimaryButton(c, "Pop").Clicked() {
				a.stashPop(e)
			}
		})
	})
	a.filesAndDiff(c, pal, key, func(f *models.CommitFile) (string, error) { return repo.StashFileDiff(e, f) })
}

func (a *App) stashAll() {
	a.prompt("Stash all changes", "", "Stash", false, func(msg string) {
		a.run("Stashing…", func(r *core.Repo) error { return r.StashAll(msg) }, func() { a.section = secStash; a.stashSel = 0 })
	})
}

func (a *App) stashPop(e *models.StashEntry) {
	a.run("Popping…", func(r *core.Repo) error { return r.StashPop(e) })
}

func (a *App) stashDrop(e *models.StashEntry) {
	a.confirm("Drop "+e.RefName()+"?", e.Name, "Drop", func() {
		a.run("Dropping…", func(r *core.Repo) error { return r.StashDrop(e) })
	})
}

// stashParts splits a stash entry's name, as git writes it, into the
// branch it was made on and its message: "On main: notes" or "WIP on main:
// 1a2b3c4 subject".
func stashParts(name string) (branch, msg string) {
	for _, prefix := range []string{"On ", "WIP on "} {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		b, m, ok := strings.Cut(rest, ": ")
		if !ok {
			continue
		}
		if prefix == "WIP on " {
			if _, subject, ok := strings.Cut(m, " "); ok {
				m = "WIP: " + subject
			}
		}
		return b, m
	}
	return "", name
}

// fresh is lazygit's age of a branch or stash, as "3h", with seconds as
// "now".
func fresh(age string) string {
	if n := strings.TrimSuffix(age, "s"); n != age && n != "" && strings.Trim(n, "0123456789") == "" {
		return "now"
	}
	return age
}
