package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
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
	ui.Row(c).Height(30).Padding(0, 8).Gap(4).AlignItems(ui.Center).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Children(func() {
		if iconButton(c, iconTerminal, "Command log (⌘L)", a.logOpen).Clicked() {
			a.logOpen = !a.logOpen
		}
		if iconButton(c, iconFolder, "Open repository… (⌘O)", false).Clicked() {
			a.openFolder()
		}
		ui.Spacer(c)
		k := ui.ButtonBase(c.Key("palette")).Height(22).Padding(0, 7).Radius(5).Center().Label("Commands").Tooltip("Commands (⌘K)")
		if k.Hovered() {
			k.Background(pal.selected)
		} else {
			k.Background(pal.pill)
		}
		k.Children(func() { ui.Text(c, "⌘K").FontSize(11).TextColor(pal.muted) })
		if k.Clicked() {
			a.openPalette()
		}
	})
	_ = tt
}

func (a *App) openFolder() {
	if a.host.OpenRepository != nil {
		a.host.OpenRepository()
	}
}

func (a *App) toolbar(c *ui.Context, pal *palette) {
	tt := c.Theme()
	bar := c.TitleBar()
	row := ui.Row(c.Key("toolbar")).Height(titleBarHeight).Shrink(0).Padding(0, 12+bar.Right, 0, 16).Gap(8).AlignItems(ui.Center).
		DragWindow().BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Background(pal.header)
	// Narrow, the branch's chip takes less room, and what it tracks moves
	// into its tooltip: the title and the buttons keep theirs.
	w := row.Bounds().W
	narrow := w > 0 && w < 760
	row.Children(func() {
		title := map[section]string{secChanges: "Changes", secHistory: "History", secBranches: "Branches", secStash: "Stashes"}[a.section]
		ui.Text(c, title).FontSize(15).FontWeight(700).SingleLine().Shrink(0)
		if s := a.snap; s != nil && s.Head != nil {
			h := s.Head
			name := h.Name
			if h.DetachedHead {
				name = h.DisplayName
			}
			tracking := ""
			if h.IsTrackingRemote() {
				tracking = "Tracks " + h.UpstreamRemote + "/" + h.UpstreamBranch
			} else if !h.DetachedHead {
				tracking = "Not published"
			}
			tip := "Checked out: " + name
			if tracking != "" {
				tip += "\n" + tracking
			}
			maxW := float32(240)
			if w > 0 && w < 820 {
				maxW = 160
			}
			chip(c, pal, iconBranch, name, tt.Text).MaxWidth(maxW).Tooltip(tip)
			sync := ""
			if h.IsTrackingRemote() {
				if h.AheadForPull != "" && h.AheadForPull != "0" {
					sync += "↑" + h.AheadForPull + " "
				}
				if h.BehindForPull != "" && h.BehindForPull != "0" {
					sync += "↓" + h.BehindForPull
				}
			}
			switch {
			case sync != "":
				ui.Text(c, strings.TrimSpace(sync)).FontSize(12).FontWeight(600).FontFeatures("tnum").
					TextColor(tt.Text.Alpha(0.7)).SingleLine().Shrink(0).Tooltip("Commits to push ↑ and to pull ↓")
			case !narrow && tracking != "":
				ui.Text(c, tracking).FontSize(12).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
			}
		}
		if a.loading || a.busy > 0 {
			ui.Spinner(c).Size(14, 14).Label("Working").Shrink(0)
			if a.doing != "" && !narrow {
				ui.Text(c, a.doing).FontSize(12).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0).MaxWidth(140)
			}
		}
		ui.Spacer(c)
		loaded := a.snap != nil
		if iconButton(c, iconUndo, "Undo (z)", false).Disabled(!loaded).Clicked() {
			a.undo(false)
		}
		if iconButton(c, iconRedo, "Redo (Z)", false).Disabled(!loaded).Clicked() {
			a.undo(true)
		}
		ui.Box(c).Width(1).Height(18).Background(pal.border).Margin(0, 4)
		if toolButton(c, pal, iconFetch, "Fetch", "Fetch (f)").Disabled(!loaded).Clicked() {
			a.fetch()
		}
		if toolButton(c, pal, iconPull, "Pull", "Pull (p)").Disabled(!loaded).Clicked() {
			a.pull()
		}
		if toolButton(c, pal, iconPush, "Push", "Push (P)").Disabled(!loaded).Clicked() {
			a.push(false)
		}
	})
}

func (a *App) fetch() { a.run("Fetching…", (*core.Repo).Fetch, func() { a.toast("Fetched", "") }) }
func (a *App) pull()  { a.run("Pulling…", (*core.Repo).Pull, func() { a.toast("Pulled", "") }) }
func (a *App) push(force bool) {
	if a.snap == nil {
		return
	}
	head, remotes := a.snap.Head, a.snap.Remotes
	a.run("Pushing…", func(r *core.Repo) error { return r.Push(head, remotes, force) }, func() { a.toast("Pushed", "") })
}

// stateBanner says what operation is in progress, with its actions.
func (a *App) stateBanner(c *ui.Context, pal *palette) {
	if a.snap == nil || a.snap.State.None() {
		return
	}
	tt := c.Theme()
	st := a.snap.State
	ui.Row(c.Key("stateBanner")).Padding(8, 14).Gap(10).AlignItems(ui.Center).Shrink(0).Background(pal.modified.Alpha(0.14)).
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
			ui.Text(c, plural(conflicts, "conflicted file")).FontSize(12).FontWeight(500).TextColor(pal.modified)
		}
		ui.Spacer(c)
		if ui.Button(c, "Abort").Clicked() {
			a.abortOperation()
		}
		if canSkip(st) && ui.Button(c, "Skip").Tooltip("Skip the commit that stopped").Clicked() {
			a.skipOperation()
		}
		if defaultButton(c, "Continue", conflicts == 0).Tooltip("Resolve the conflicts first").Clicked() {
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
	if a.snap == nil {
		return
	}
	st := a.snap.State
	a.run("Continuing…", func(r *core.Repo) error { return r.ContinueOperation(st) })
}

func (a *App) skipOperation() {
	if a.snap == nil {
		return
	}
	st := a.snap.State
	a.run("Skipping…", func(r *core.Repo) error { return r.SkipOperation(st) })
}

// operationNoun is what the operation in progress is called: "merge",
// "rebase", "cherry-pick" or "revert".
func (a *App) operationNoun(st models.WorkingTreeState) string {
	return strings.TrimPrefix(st.OperationName(a.repoTr()), "the ")
}

func (a *App) abortOperation() {
	if a.snap == nil {
		return
	}
	st := a.snap.State
	noun := a.operationNoun(st)
	a.confirm("Abort the "+noun+"?", "The repository goes back to how it was before the "+noun+" started.", "Abort "+titleCase(noun), func() {
		a.run("Aborting…", func(r *core.Repo) error { return r.AbortOperation(st) })
	})
}

// statusBar shows the keys of the pane with the focus: as many as fit
// whole, the rest in the palette (?).
func (a *App) statusBar(c *ui.Context, pal *palette) {
	const pad, gap = 12, 14
	bar := ui.Row(c.Key("statusBar")).Height(30).Shrink(0).Padding(0, pad).Gap(gap).AlignItems(ui.Center).Clip().
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(pal.header)
	room := bar.Bounds().W - 2*pad
	bar.Children(func() {
		hints := a.hints()
		width := func(h hint) float32 {
			kw, _ := c.MeasureText(0, ui.Span{Text: h.key, Font: codeFont, Size: 11})
			lw, _ := c.MeasureText(0, ui.Span{Text: h.label, Size: 11})
			return kw + 10 + 5 + lw + gap
		}
		// The last, "? Commands", which leads to the rest, always shows.
		last := hints[len(hints)-1]
		used := width(last)
		show := func(h hint) {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, h.key).Font(codeFont).FontSize(11).Padding(0, 5).Radius(4).Background(pal.pill).Shrink(0)
				ui.Text(c, h.label).FontSize(11).TextColor(pal.muted).SingleLine().Shrink(0)
			})
		}
		for _, h := range hints[:len(hints)-1] {
			w := width(h)
			if room > 0 && used+w > room {
				break
			}
			used += w
			show(h)
		}
		show(last)
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
		ui.Icon(c, svg).FontSize(12).Shrink(0)
		ui.Text(c, label).FontSize(12).FontWeight(600).SingleLine().Shrink(1).MinWidth(0)
	})
}

func toolButton(c *ui.Context, pal *palette, svg *ui.SVG, label, tip string) ui.Element {
	tt := c.Theme()
	b := ui.ButtonBase(c.Key(label)).Height(28).Padding(0, 10).Gap(6).Radius(6).AlignItems(ui.Center).Shrink(0).
		TextColor(tt.Text).Label(label).Tooltip(tip)
	switch {
	case b.Pressed():
		b.Background(pal.selected)
	case b.Hovered():
		b.Background(pal.hover)
	}
	b.Children(func() {
		ui.Icon(c, svg).FontSize(14)
		ui.Text(c, label).FontSize(12).FontWeight(500)
	})
	return b
}

// headerButton is a small button, for the bars over the panes.
func headerButton(c *ui.Context, label string) ui.Element {
	return ui.Button(c, label).Height(24).Padding(0, 10).FontSize(12)
}

// toasts shows the toasts as cards at the bottom right, over the status
// bar, with an icon for their kind.
func toasts(c *ui.Context, pal *palette) {
	tt := c.Theme()
	w, _ := c.Size()
	ui.ToastViewportBase(c, func(viewport ui.Element, list []ui.Toast) {
		// Bottom right, clear of the status bar and the action bars over
		// it (the composer's, the selection's), over content only.
		viewport.Padding(0, 16, 30+54+12, 16).AlignItems(ui.End).Gap(8)
		for _, ts := range list {
			p := ui.ToastBase(c, ts)
			bg := pal.raised
			icon, color := iconCheck, pal.added
			switch ts.Type {
			case "error":
				icon, color = iconWarning, pal.deleted
			case "warning":
				icon, color = iconWarning, pal.modified
			case "info":
				icon, color = iconCommand, tt.Accent
			}
			p.Root.Row().Width(360).MaxWidth(w-32).Gap(10).Padding(11, 12, 12, 12).Radius(10).AlignItems(ui.Start).
				Background(bg).Border(1, pal.border).Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.16))
			p.Root.Children(func() {
				ui.Icon(c, icon).FontSize(15).TextColor(color).Shrink(0).Margin(1, 0, 0, 0)
				ui.Column(c).Gap(3).Grow(1).Shrink(1).MinWidth(0).Children(func() {
					ui.Text(c, ts.Title).FontSize(13).FontWeight(600)
					switch {
					case ts.Description == "":
					case ts.Type == "error":
						// git's whole message, its hint last: it scrolls
						// rather than being cut short.
						ui.Scroll(c.Key("detail")).MaxHeight(128).Children(func() {
							ui.Text(c, ts.Description).Font(codeFont).FontSize(11).TextColor(pal.muted).Selectable()
						})
					default:
						ui.Text(c, ts.Description).FontSize(12).TextColor(pal.muted).MaxLines(4).Selectable()
					}
				})
				p.CloseButton().Size(18, 18).Radius(4).Center().Label("Close").TextColor(pal.muted).Shrink(0).
					Children(func() { ui.Text(c, "✕").FontSize(10) })
			})
		}
	})
}

func iconButton(c *ui.Context, svg *ui.SVG, tip string, on bool) ui.Element {
	tt := c.Theme()
	pal := paletteFor(tt)
	b := ui.ButtonBase(c.Key(tip)).Size(28, 24).Radius(6).Center().Shrink(0).TextColor(pal.muted).Label(tip).Tooltip(tip)
	switch {
	case on || b.Pressed():
		b.Background(pal.selected).TextColor(tt.Text)
	case b.Hovered():
		b.Background(pal.hover)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(15) })
	return b
}

// defaultButton is the pane's main action: in the accent while it can be
// done, a plain button while it can't, as AppKit's default button.
func defaultButton(c *ui.Context, label string, enabled bool) ui.Element {
	if enabled {
		return ui.PrimaryButton(c.Key(label), label)
	}
	return ui.Button(c.Key(label), label).Disabled(true)
}

// small builds controls at the small size, for the bars over the panes.
func small(c *ui.Context, build func()) {
	th := *c.Theme()
	th.Spacing, th.FontSize = 3, 12
	ui.Themed(c, &th).Row().AlignItems(ui.Center).Gap(6).Shrink(0).Children(build)
}
