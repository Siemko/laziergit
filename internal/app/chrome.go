package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

func (a *App) sidebar(c *ui.Context, pal *palette) {
	tt := c.Theme()
	vibrant := c.Vibrancy()
	ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()
	ui.Column(c).Padding(0, 14, 12, 14).Gap(2).Shrink(0).Children(func() {
		ui.Text(c, a.repo.Name()).FontSize(15).Bold().SingleLine()
		ui.Text(c, abbreviateHome(filepath.Dir(a.repo.Path))).FontSize(11).TextColor(pal.muted).SingleLine()
	})
	sel := string(a.section)
	counts := map[section]int{}
	if a.snap != nil {
		counts[secChanges] = len(a.snap.Files)
		counts[secStash] = len(a.snap.Stash)
		counts[secBranches] = len(a.snap.Branches)
	}
	item := func(s section, svg *ui.SVG, label string, n int, key string) {
		ui.SidebarItem(c, string(s), svg, label).Tooltip(label + " (" + key + ")").Children(func() {
			if n > 0 {
				ui.Badge(c, fmt.Sprint(n))
			}
		})
	}
	list := ui.Sidebar(c, &sel, func() {
		ui.SidebarSection(c, "Workspace", nil, func() {
			item(secChanges, iconChanges, "Changes", counts[secChanges], "1")
			item(secStash, iconStash, "Stashes", counts[secStash], "4")
		})
		ui.SidebarSection(c, "Repository", nil, func() {
			item(secHistory, iconHistory, "History", 0, "2")
			item(secBranches, iconBranch, "Branches", counts[secBranches], "3")
		})
		if a.snap != nil && len(a.snap.Remotes) > 0 {
			ui.SidebarSection(c, "Remotes", nil, func() {
				for _, r := range a.snap.Remotes {
					ui.SidebarItem(c, "remote:"+r.Name, iconRemote, r.Name).Tooltip(strings.Join(r.Urls, "\n"))
				}
			})
		}
	}).Grow(1).Label("Sections")
	if vibrant {
		list.Background(ui.Transparent)
	}
	if list.Changed() {
		if strings.HasPrefix(sel, "remote:") {
			a.section = secBranches
		} else {
			a.section = section(sel)
		}
	}
	ui.Row(c).Padding(8, 10).Gap(6).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {
		if iconButton(c, iconTerminal, "Command log (⌘L)", a.logOpen).Clicked() {
			a.logOpen = !a.logOpen
		}
		if iconButton(c, iconFolder, "Open repository… (⌘O)", false).Clicked() {
			a.openFolder()
		}
		ui.Spacer(c)
		ui.Text(c, "⌘K").FontSize(11).TextColor(pal.muted).Padding(2, 6).Radius(5).Background(pal.pill).Tooltip("Commands")
	})
	_ = tt
}

func (a *App) openFolder() {
	if a.host.OpenFolder == nil {
		return
	}
	if dir := a.host.OpenFolder(); dir != "" {
		if err := a.Open(dir); err != nil {
			a.toastErr(err)
		}
	}
}

func (a *App) toolbar(c *ui.Context, pal *palette) {
	tt := c.Theme()
	bar := c.TitleBar()
	ui.Row(c).Height(titleBarHeight).Shrink(0).Padding(0, 12+bar.Right, 0, 16).Gap(8).AlignItems(ui.Center).
		DragWindow().BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Background(pal.header).Children(func() {
		title := map[section]string{secChanges: "Changes", secHistory: "History", secBranches: "Branches", secStash: "Stashes"}[a.section]
		ui.Text(c, title).FontSize(15).Bold().SingleLine()
		if s := a.snap; s != nil && s.Head != nil {
			h := s.Head
			name := h.Name
			if h.DetachedHead {
				name = h.DisplayName
			}
			chip(c, pal, iconBranch, name, tt.Text).Tooltip("Checked out")
			if h.IsTrackingRemote() {
				sync := ""
				if h.AheadForPull != "" && h.AheadForPull != "0" {
					sync += "↑" + h.AheadForPull + " "
				}
				if h.BehindForPull != "" && h.BehindForPull != "0" {
					sync += "↓" + h.BehindForPull
				}
				if sync != "" {
					ui.Text(c, strings.TrimSpace(sync)).Font(codeFont).FontSize(12).TextColor(pal.modified)
				} else {
					ui.Text(c, "✓ "+h.UpstreamRemote+"/"+h.UpstreamBranch).FontSize(12).TextColor(pal.muted).SingleLine().Shrink(1)
				}
			} else if !h.DetachedHead {
				ui.Text(c, "not published").FontSize(12).TextColor(pal.muted)
			}
		}
		if a.loading || a.busy > 0 {
			ui.Spinner(c).Size(14, 14).Label("Working")
			if a.doing != "" {
				ui.Text(c, a.doing).FontSize(12).TextColor(pal.muted).SingleLine()
			}
		}
		ui.Spacer(c)
		if toolButton(c, pal, iconFetch, "Fetch", "Fetch (f)").Clicked() {
			a.fetch()
		}
		if toolButton(c, pal, iconPull, "Pull", "Pull (p)").Clicked() {
			a.pull()
		}
		if toolButton(c, pal, iconPush, "Push", "Push (P)").Clicked() {
			a.push(false)
		}
	})
}

func (a *App) fetch() { a.run("Fetching…", a.repo.Fetch, func() { a.toast("Fetched", "") }) }
func (a *App) pull()  { a.run("Pulling…", a.repo.Pull, func() { a.toast("Pulled", "") }) }
func (a *App) push(force bool) {
	head, remotes := a.snap.Head, a.snap.Remotes
	a.run("Pushing…", func() error { return a.repo.Push(head, remotes, force) }, func() { a.toast("Pushed", "") })
}

// stateBanner says what operation is in progress, with its actions.
func (a *App) stateBanner(c *ui.Context, pal *palette) {
	if a.snap == nil || a.snap.State.None() {
		return
	}
	tt := c.Theme()
	st := a.snap.State
	ui.Row(c).Padding(8, 14).Gap(10).AlignItems(ui.Center).Shrink(0).Background(pal.modified.Alpha(0.14)).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Icon(c, iconWarning).FontSize(15).TextColor(pal.modified)
		ui.Text(c, st.Title(a.repoTr())).FontWeight(600)
		conflicts := 0
		for _, f := range a.snap.Files {
			if f.HasMergeConflicts {
				conflicts++
			}
		}
		if conflicts > 0 {
			ui.Text(c, plural(conflicts, "conflicted file")).FontSize(12).TextColor(pal.conflict)
		}
		ui.Spacer(c)
		if ui.Button(c, "Abort").Clicked() {
			a.abortOperation()
		}
		if canSkip(st) && ui.Button(c, "Skip").Tooltip("Skip the commit that stopped").Clicked() {
			a.skipOperation()
		}
		if ui.PrimaryButton(c, "Continue").Disabled(conflicts > 0).Tooltip("Resolve the conflicts first").Clicked() {
			a.continueOperation()
		}
		_ = tt
	})
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func canSkip(st models.WorkingTreeState) bool {
	return st.Rebasing || st.CherryPicking || st.Reverting
}

func (a *App) continueOperation() {
	st := a.snap.State
	a.run("Continuing…", func() error { return a.repo.ContinueOperation(st) })
}

func (a *App) skipOperation() {
	st := a.snap.State
	a.run("Skipping…", func() error { return a.repo.SkipOperation(st) })
}

// operationNoun is what the operation in progress is called: "merge",
// "rebase", "cherry-pick" or "revert".
func (a *App) operationNoun(st models.WorkingTreeState) string {
	return strings.TrimPrefix(st.OperationName(a.repoTr()), "the ")
}

func (a *App) abortOperation() {
	st := a.snap.State
	noun := a.operationNoun(st)
	a.confirm("Abort the "+noun+"?", "The repository goes back to how it was before the "+noun+" started.", "Abort "+titleCase(noun), func() {
		a.run("Aborting…", func() error { return a.repo.AbortOperation(st) })
	})
}

func (a *App) statusBar(c *ui.Context, pal *palette) {
	ui.Row(c).Height(26).Shrink(0).Padding(0, 12).Gap(14).AlignItems(ui.Center).Clip().
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(pal.header).Children(func() {
		for _, h := range a.hints() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, h.key).Font(codeFont).FontSize(10).Padding(1, 5).Radius(4).Background(pal.pill).Shrink(0)
				ui.Text(c, h.label).FontSize(11).TextColor(pal.muted).SingleLine().Shrink(0)
			})
		}
		ui.Spacer(c)
		if n := len(a.log); n > 0 {
			last := a.log[n-1].text
			ui.Text(c, last).Font(codeFont).FontSize(11).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0).MaxWidth(360).Tooltip(last)
		}
	})
}

func (a *App) logView(c *ui.Context, pal *palette) {
	tt := c.Theme()
	ui.Column(c).Height(180).Shrink(0).Background(tt.Background).Children(func() {
		paneHeader(c, pal, "Command log", func() {
			if ui.Button(c, "Clear").Clicked() {
				a.log = nil
			}
		})
		ui.List(c, &a.logList, len(a.log), func(i int) {
			e := a.log[i]
			color := pal.muted
			prefix := "  "
			if e.cmd {
				color, prefix = tt.Text, "$ "
			}
			if strings.HasPrefix(e.text, "error: ") {
				color = pal.deleted
			}
			ui.Row(c).Padding(2, 14).Gap(10).Children(func() {
				ui.Text(c, e.at.Format("15:04:05")).Font(codeFont).FontSize(11).TextColor(pal.muted)
				ui.Text(c, prefix+e.text).Font(codeFont).FontSize(11).TextColor(color).Selectable().Shrink(1).MinWidth(0)
			})
		}).Grow(1)
	})
}

// chip is a label in a pill, as the branch.
func chip(c *ui.Context, pal *palette, svg *ui.SVG, label string, color ui.Color) ui.Element {
	return ui.Row(c).Gap(5).Padding(3, 9).Radius(12).Background(pal.pill).AlignItems(ui.Center).
		TextColor(color).Shrink(1).MinWidth(0).MaxWidth(260).Children(func() {
		ui.Icon(c, svg).FontSize(12)
		ui.Text(c, label).FontSize(12).FontWeight(600).SingleLine()
	})
}

func toolButton(c *ui.Context, pal *palette, svg *ui.SVG, label, tip string) ui.Element {
	tt := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Height(28).Padding(0, 10).Gap(6).Radius(7).AlignItems(ui.Center).
		TextColor(tt.Text).Label(label).Tooltip(tip)
	if b.Hovered() {
		b.Background(pal.hover)
	}
	b.Children(func() {
		ui.Icon(c, svg).FontSize(14)
		ui.Text(c, label).FontSize(12).FontWeight(500)
	})
	return b
}

func iconButton(c *ui.Context, svg *ui.SVG, tip string, on bool) ui.Element {
	tt := c.Theme()
	pal := paletteFor(tt)
	b := ui.ButtonBase(c.Key(tip)).Size(28, 26).Radius(6).Center().TextColor(pal.muted).Label(tip).Tooltip(tip)
	if on {
		b.Background(pal.selected).TextColor(tt.Text)
	} else if b.Hovered() {
		b.Background(pal.hover)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(15) })
	return b
}
