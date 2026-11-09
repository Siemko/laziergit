package app

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
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
		if ui.Button(c, "New Branch…").Tooltip("New branch (n)").Clicked() {
			a.newBranch()
		}
	})
	a.branchesList.Selected = &a.branchSel
	a.branchesList.Key = func(i int) any { return branches[i].Name }
	a.branchesList.Label = func(i int) string { return branches[i].Name }
	list := ui.List(c.Key("branches"), &a.branchesList, len(branches), func(i int) {
		a.branchRow(c, pal, branches[i], i == a.branchSel)
	}).Grow(1).FocusBind(&a.pane, paneList)
	if list.Submitted() {
		a.checkout(branches[a.branchSel])
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
				ui.Text(c, "↑"+ahead).Font(codeFont).FontSize(11).TextColor(tint(sel, tt, pal.modified))
			}
			if behind := b.BehindForPull; behind != "" && behind != "0" {
				ui.Text(c, "↓"+behind).Font(codeFont).FontSize(11).TextColor(tint(sel, tt, pal.modified))
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

func (a *App) branchDetail(c *ui.Context, pal *palette) {
	b := a.selectedBranch()
	if b == nil {
		emptyState(c, pal, iconBranch, "No branches", "")
		return
	}
	tt := c.Theme()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Padding(24, 28).Gap(14).MaxWidth(560).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, iconBranch).FontSize(22).TextColor(tt.Accent)
				ui.Text(c, b.Name).FontSize(20).Bold().Selectable()
				if b.Head {
					ui.Text(c, "HEAD").FontSize(10).FontWeight(700).TextColor(tt.AccentText).Padding(2, 6).Radius(4).Background(tt.Accent)
				}
			})
			card := func(label, value string) {
				ui.Row(c).Gap(12).Children(func() {
					ui.Text(c, label).FontSize(12).TextColor(pal.muted).Width(110)
					ui.Text(c, value).FontSize(12).Selectable().Grow(1)
				})
			}
			ui.Column(c).Gap(8).Padding(14).Radius(10).Background(pal.pane).Border(1, pal.border).Children(func() {
				if b.IsTrackingRemote() {
					card("Upstream", b.UpstreamRemote+"/"+b.UpstreamBranch)
					card("Ahead / behind", fmt.Sprintf("%s / %s", orZero(b.AheadForPull), orZero(b.BehindForPull)))
				} else {
					card("Upstream", "none — Push publishes it")
				}
				if r := recency(b); r != "" {
					card("Last checked out", r+" ago")
				}
			})
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if ui.PrimaryButton(c, "Checkout").Disabled(b.Head).Clicked() {
					a.checkout(b)
				}
				if ui.Button(c, "Merge into current").Disabled(b.Head).Clicked() {
					a.merge(b)
				}
				if ui.Button(c, "Rebase current onto").Disabled(b.Head).Clicked() {
					a.rebaseOnto(b)
				}
				if ui.Button(c, "Delete…").Disabled(b.Head).Clicked() {
					a.deleteBranch(b)
				}
			})
		})
	})
}

// recency is how long ago a branch was checked out; lazygit's "*" for the
// one checked out says nothing more than its dot.
func recency(b *models.Branch) string {
	if b.Head || b.Recency == "*" {
		return ""
	}
	return b.Recency
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
	a.run("Checking out "+b.Name+"…", func() error { return a.repo.Checkout(b.Name) }, func() { a.toast("Checked out "+b.Name, "") })
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
		a.run("Creating "+name+"…", func() error { return a.repo.NewBranch(name, base) }, func() { a.branchSel = 0 })
	})
}

func (a *App) merge(b *models.Branch) {
	a.confirm("Merge "+b.Name+" into "+a.headName()+"?", "", "Merge", func() {
		a.run("Merging…", func() error { return a.repo.Merge(b.Name) })
	})
}

func (a *App) rebaseOnto(b *models.Branch) {
	a.confirm("Rebase "+a.headName()+" onto "+b.Name+"?", "", "Rebase", func() {
		a.run("Rebasing…", func() error { return a.repo.RebaseOnto(b.Name) })
	})
}

func (a *App) deleteBranch(b *models.Branch) {
	a.confirm("Delete branch "+b.Name+"?", "Unmerged commits on it are force-deleted.", "Delete", func() {
		a.run("Deleting…", func() error { return a.repo.DeleteBranch(b.Name, true) })
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
		if ui.Button(c, "Stash Changes").Disabled(len(a.snap.Files) == 0).Tooltip("Stash all changes (s)").Clicked() {
			a.stashAll()
		}
	})
	a.stashList.Selected = &a.stashSel
	a.stashList.Key = func(i int) any { return entries[i].Hash }
	a.stashList.Label = func(i int) string { return entries[i].Name }
	ui.List(c.Key("stash"), &a.stashList, len(entries), func(i int) {
		e := entries[i]
		sel := i == a.stashSel
		tt := c.Theme()
		ui.Row(c).Height(rowHeight+4).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, e.RefName()).Font(codeFont).FontSize(11).TextColor(tint(sel, tt, pal.hash)).Shrink(0)
			ui.Text(c, e.Name).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			ui.Text(c, e.Recency).FontSize(11).TextColor(tint(sel, tt, pal.muted))
		}).ContextMenu(func(m *ui.Menu) {
			if m.Item("Pop").Shortcut(0, ui.KeyG).Chosen() {
				a.stashPop(e)
			}
			if m.Item("Apply").Shortcut(0, ui.KeySpace).Chosen() {
				a.run("Applying…", func() error { return a.repo.StashApply(e) })
			}
			m.Separator()
			if m.Item("Drop…").Shortcut(0, ui.KeyD).Chosen() {
				a.stashDrop(e)
			}
		})
	}).Grow(1).FocusBind(&a.pane, paneList).Children(func() {
		if len(entries) == 0 {
			emptyState(c, pal, iconStash, "No stashes", "Stash changes to set them aside.")
		}
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
	ui.Row(c).Padding(12, 16).Gap(8).AlignItems(ui.Center).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Text(c, e.Name).FontSize(14).Bold().SingleLine().Shrink(1)
		ui.Spacer(c)
		if ui.Button(c, "Apply").Clicked() {
			a.run("Applying…", func() error { return a.repo.StashApply(e) })
		}
		if ui.PrimaryButton(c, "Pop").Clicked() {
			a.stashPop(e)
		}
	})
	a.filesAndDiff(c, pal, key, func(f *models.CommitFile) (string, error) { return repo.StashFileDiff(e, f) })
}

func (a *App) stashAll() {
	a.prompt("Stash all changes", "", "Stash", false, func(msg string) {
		a.run("Stashing…", func() error { return a.repo.StashAll(msg) }, func() { a.section = secStash; a.stashSel = 0 })
	})
}

func (a *App) stashPop(e *models.StashEntry) {
	a.run("Popping…", func() error { return a.repo.StashPop(e) })
}

func (a *App) stashDrop(e *models.StashEntry) {
	a.confirm("Drop "+e.RefName()+"?", e.Name, "Drop", func() {
		a.run("Dropping…", func() error { return a.repo.StashDrop(e) })
	})
}
