package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"github.com/Siemko/laziergit/internal/core"
)

const (
	graphCol    = 14 // the width of a column of the graph
	graphMaxCol = 10 // columns past this are cut off
)

func (a *App) selectedCommit() *models.Commit {
	if a.snap == nil || a.commitSel < 0 || a.commitSel >= len(a.snap.Commits) {
		return nil
	}
	return a.snap.Commits[a.commitSel]
}

// graphWidth is the width of a graph's widest rows, up to graphMaxCol
// columns.
func graphWidth(graph [][]core.Pipe) float32 {
	cols := int16(0)
	for _, row := range graph {
		for _, p := range row {
			if p.Visible() {
				cols = max(cols, p.From+1, p.To+1)
			}
		}
	}
	return float32(min(cols, graphMaxCol))*graphCol + 6
}

func (a *App) historyList(c *ui.Context, pal *palette) {
	commits := a.snap.Commits
	n := len(commits)
	title := fmt.Sprintf("%d commits", n)
	if n >= core.CommitLimit {
		title = fmt.Sprintf("Latest %d commits", n)
	}
	paneHeader(c, pal, title, nil)
	gw := graphWidth(a.snap.Graph)
	a.commitsList.Selected = &a.commitSel
	a.commitsList.Key = func(i int) any { return commits[i].Hash() }
	a.commitsList.Label = func(i int) string { return commits[i].Name }
	list := ui.List(c.Key("commits"), &a.commitsList, n, func(i int) {
		a.commitRow(c, pal, commits[i], a.snap.Graph[i], gw, i == a.commitSel).
			ContextMenu(func(m *ui.Menu) { a.commitMenu(m, i) })
	}).Grow(1).FocusBind(&a.pane, paneList)
	if list.Submitted() {
		a.pane = paneFiles
	}
}

// commitRow is a commit of a log: its graph, refs, subject, author, age
// and hash, and whether it is copied to cherry-pick.
func (a *App) commitRow(c *ui.Context, pal *palette, cm *models.Commit, row []core.Pipe, gw float32, sel bool) ui.Element {
	tt := c.Theme()
	copied := a.isCopied(cm)
	return ui.Row(c).Height(rowHeight).Padding(0, 10, 0, 4).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Width(gw).Height(rowHeight).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			drawGraphRow(p, r, row, cm, pal, tt, sel)
		})
		ui.Row(c).Gap(5).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
			if cm.IsTODO() {
				ui.Text(c, strings.ToLower(cm.Action.String())).Font(codeFont).FontSize(10).FontWeight(700).
					TextColor(tint(sel, tt, pal.modified)).Padding(1, 6).Radius(4).Background(pal.modified.Alpha(0.14)).Shrink(0)
			}
			refs := refsOf(cm)
			shown := refs[:min(len(refs), 2)]
			for _, ref := range shown {
				color := pal.renamed
				switch ref.kind {
				case refHead:
					color = tt.Accent
				case refTag:
					color = pal.modified
				}
				chip := ui.Text(c, ref.label).FontSize(10).FontWeight(600).Padding(1, 6).Radius(4).
					SingleLine().Shrink(1).MinWidth(min(48, float32(len(ref.label))*6+12)).MaxWidth(110).Tooltip(ref.full)
				if sel {
					chip.TextColor(tt.Accent).Background(tt.AccentText.Alpha(0.9))
				} else {
					chip.TextColor(color).Background(color.Alpha(0.13))
				}
			}
			if n := len(refs) - len(shown); n > 0 {
				ui.Textf(c, "+%d", n).FontSize(10).FontWeight(600).TextColor(tint(sel, tt, pal.muted)).Shrink(0).
					Tooltip(refsTooltip(refs[len(shown):]))
			}
			ui.Text(c, cm.Name).FontSize(13).SingleLine().Grow(1).Shrink(1).MinWidth(110)
		})
		if copied {
			ui.Text(c, "copied").FontSize(10).FontWeight(700).Padding(1, 6).Radius(4).Shrink(0).
				TextColor(tint(sel, tt, pal.conflict)).Background(pal.conflict.Alpha(0.14)).Tooltip("Copied to cherry-pick (V pastes)")
		}
		ui.Text(c, initials(cm.AuthorName)).FontSize(10).FontWeight(600).TextColor(tint(sel, tt, pal.muted)).Tooltip(cm.AuthorName).Shrink(0)
		ui.Text(c, relTime(cm.UnixTimestamp)).FontSize(11).TextColor(tint(sel, tt, pal.muted)).Width(30).TextAlign(ui.End).Shrink(0)
		hash := ui.Text(c, cm.ShortHash()).Font(codeFont).FontSize(11).Shrink(0)
		switch cm.Status {
		case models.StatusUnpushed:
			hash.TextColor(tint(sel, tt, pal.unpushed)).Tooltip("Not pushed")
		case models.StatusMerged:
			hash.TextColor(tint(sel, tt, pal.added)).Tooltip("In a main branch")
		default:
			hash.TextColor(tint(sel, tt, pal.hash))
		}
	})
}

func (a *App) commitMenu(m *ui.Menu, i int) {
	a.commitSel = i
	commits := a.snap.Commits
	if m.Item(copyLabel(a.isCopied(commits[i]))).Shortcut(ui.Shift, ui.KeyC).Chosen() {
		a.toggleCopy("HEAD", commits, i)
	}
	if len(a.copied) > 0 && m.Item("Paste Copied Commits…").Shortcut(ui.Shift, ui.KeyV).Chosen() {
		a.pasteCopied()
	}
	m.Separator()
	if m.Item("Reword…").Shortcut(0, ui.KeyR).Chosen() {
		a.reword(i)
	}
	if m.Item("Fixup into Commit Below").Shortcut(0, ui.KeyF).Chosen() {
		a.fixup(i)
	}
	if m.Item("Move Up").Shortcut(ui.Ctrl, ui.KeyK).Chosen() {
		a.moveCommit(i, -1)
	}
	if m.Item("Move Down").Shortcut(ui.Ctrl, ui.KeyJ).Chosen() {
		a.moveCommit(i, 1)
	}
	m.Separator()
	if m.Item("Drop Commit…").Shortcut(0, ui.KeyD).Chosen() {
		a.drop(i)
	}
}

type refKind int

const (
	refBranch refKind = iota
	refHead           // the branch checked out
	refTag
)

type ref struct {
	label, full string
	kind        refKind
}

// refsOf is the branches and tags at a commit, from git's decoration:
// "HEAD -> main, tag: v1, origin/main".
func refsOf(c *models.Commit) []ref {
	s := strings.TrimSpace(c.ExtraInfo)
	s = strings.TrimSuffix(strings.TrimPrefix(s, "("), ")")
	if s == "" {
		return nil
	}
	var out []ref
	for _, part := range strings.Split(s, ", ") {
		switch {
		case strings.HasPrefix(part, "HEAD -> "):
			out = append(out, ref{strings.TrimPrefix(part, "HEAD -> "), part, refHead})
		case part == "HEAD":
			out = append(out, ref{"HEAD", part, refHead})
		case strings.HasPrefix(part, "tag: "):
			out = append(out, ref{strings.TrimPrefix(part, "tag: "), part, refTag})
		default:
			out = append(out, ref{part, part, refBranch})
		}
	}
	return out
}

func refsTooltip(refs []ref) string {
	var names []string
	for _, r := range refs {
		names = append(names, r.full)
	}
	return strings.Join(names, "\n")
}

func initials(name string) string {
	var out []rune
	for _, f := range strings.Fields(name) {
		out = append(out, []rune(strings.ToUpper(f))[0])
		if len(out) == 2 {
			break
		}
	}
	return string(out)
}

// drawGraphRow draws a row of the commit graph: lazygit's pipes as curves,
// and the commit as a dot.
func drawGraphRow(p *ui.Painter, r ui.Rect, row []core.Pipe, cm *models.Commit, pal *palette, tt *ui.Theme, sel bool) {
	x := func(col int16) float32 { return r.X + 4 + float32(col)*graphCol + graphCol/2 }
	top, mid, bottom := r.Y, r.Y+r.H/2, r.Y+r.H
	color := func(pp core.Pipe) ui.Color { return pal.graph[int(pp.Color)%len(pal.graph)] }
	curve := func(x0, y0, x1, y1 float32) *ui.Path {
		path := (&ui.Path{}).MoveTo(x0, y0)
		if x0 == x1 {
			return path.LineTo(x1, y1)
		}
		my := (y0 + y1) / 2
		return path.CubeTo(x0, my, x1, my, x1, y1)
	}
	for _, pp := range row {
		if !pp.Visible() || pp.From >= graphMaxCol && pp.To >= graphMaxCol {
			continue
		}
		var path *ui.Path
		switch pp.Kind {
		case core.Terminates:
			path = curve(x(pp.From), top, x(pp.To), mid)
		case core.Starts:
			path = curve(x(pp.From), mid, x(pp.To), bottom)
		case core.Continues:
			path = curve(x(pp.From), top, x(pp.To), bottom)
		}
		p.StrokePath(path, 1.6, color(pp))
	}
	col := core.CommitColumn(row)
	if col >= graphMaxCol {
		return
	}
	var dot ui.Color
	for _, pp := range row {
		if pp.Kind == core.Starts && pp.From == col {
			dot = color(pp)
			break
		}
	}
	cx := x(col)
	// The dot is ringed in the row's background, so the pipes stop short
	// of it: the accent on the row chosen.
	ring := pal.pane
	if sel {
		ring = tt.Accent
	}
	if cm.IsMerge() {
		p.FillPath((&ui.Path{}).Circle(cx, mid, 5), ring)
		p.StrokePath((&ui.Path{}).Circle(cx, mid, 3.6), 1.8, dot)
		return
	}
	p.FillPath((&ui.Path{}).Circle(cx, mid, 5.2), ring)
	p.FillPath((&ui.Path{}).Circle(cx, mid, 3.8), dot)
}

func (a *App) commitDetail(c *ui.Context, pal *palette) {
	cm := a.selectedCommit()
	if cm == nil {
		emptyState(c, pal, iconHistory, "No commits yet", "")
		return
	}
	repo := a.repo
	key := "commit:" + cm.Hash()
	a.detailOf(key, func() ([]*models.CommitFile, string, error) {
		files, err := repo.CommitFiles(cm)
		if err != nil {
			return nil, "", err
		}
		msg, _ := repo.CommitMessage(cm)
		return files, msg, nil
	})
	ui.Column(c).Padding(14, 16).Gap(6).Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		subject, body, _ := strings.Cut(strings.TrimSpace(a.detailMsg), "\n")
		if subject == "" {
			subject = cm.Name
		}
		ui.Text(c, subject).FontSize(15).Bold().Selectable()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cm.AuthorName).FontSize(12).FontWeight(600)
			ui.Text(c, cm.AuthorEmail).FontSize(12).TextColor(pal.muted).SingleLine().Shrink(1)
			ui.Spacer(c)
			ui.Text(c, cm.Hash()[:min(12, len(cm.Hash()))]).Font(codeFont).FontSize(11).TextColor(pal.hash).Selectable()
			ui.Text(c, relTime(cm.UnixTimestamp)+" ago").FontSize(12).TextColor(pal.muted)
		})
		if body = strings.TrimSpace(body); body != "" {
			// A long message scrolls in a few lines' room.
			ui.Scroll(c.Key("body")).MaxHeight(84).Children(func() {
				ui.Text(c, body).FontSize(12).TextColor(pal.muted).Selectable()
			})
		}
	})
	a.filesAndDiff(c, pal, key, func(f *models.CommitFile) (string, error) { return repo.CommitFileDiff(cm, f) })
}

// detailOf loads the files (and message) of the commit or stash entry key
// names, when it changes.
func (a *App) detailOf(key string, load func() ([]*models.CommitFile, string, error)) {
	if key == a.detailKey {
		return
	}
	a.detailKey = key
	a.detailFiles, a.detailMsg, a.detailFileSel = nil, "", 0
	a.detailFilesList = ui.ListState{}
	a.host.Go(func() {
		files, msg, err := load()
		a.host.Post(func() {
			if a.detailKey != key {
				return
			}
			if err != nil {
				a.toastErr(err)
				return
			}
			a.detailFiles, a.detailMsg = files, msg
		})
	})
}

// filesAndDiff is the files of a commit or stash entry beside the diff of
// the one chosen.
func (a *App) filesAndDiff(c *ui.Context, pal *palette, key string, diff func(f *models.CommitFile) (string, error)) {
	files := a.detailFiles
	ui.Split(c, &a.filesW, func() {
		ui.Column(c).Fill().Children(func() {
			paneHeader(c, pal, plural(len(files), "file"), nil)
			a.detailFilesList.Selected = &a.detailFileSel
			a.detailFilesList.Key = func(i int) any { return files[i].Path }
			a.detailFilesList.Label = func(i int) string { return files[i].Path }
			ui.List(c.Key("detailFiles"), &a.detailFilesList, len(files), func(i int) {
				f := files[i]
				sel := i == a.detailFileSel
				tt := c.Theme()
				letter, color := commitFileLetter(pal, f.ChangeStatus)
				dir, name := splitPath(f.Path)
				ui.Row(c).Height(26).Padding(0, 10).Gap(7).AlignItems(ui.Center).Children(func() {
					ui.Text(c, letter).Font(codeFont).FontSize(11).FontWeight(700).TextColor(tint(sel, tt, color)).Width(12)
					ui.Text(c, name).FontSize(12).SingleLine().Shrink(0)
					ui.Text(c, dir).FontSize(11).TextColor(tint(sel, tt, pal.muted)).SingleLine().Shrink(1).MinWidth(0)
				})
			}).Grow(1).FocusBind(&a.pane, paneFiles)
		})
	}, func() {
		ui.Column(c).Fill().Children(func() {
			if a.detailFileSel < 0 || a.detailFileSel >= len(files) {
				emptyState(c, pal, iconChanges, "", "")
				return
			}
			f := files[a.detailFileSel]
			paneHeader(c, pal, f.Path, func() { a.layoutToggle(c) })
			a.diffView(c, pal, key+":"+f.Path, func() (string, error) { return diff(f) })
		})
	}).SplitReset(240).Grow(1).MinHeight(0)
}

func commitFileLetter(pal *palette, status string) (string, ui.Color) {
	switch {
	case strings.HasPrefix(status, "A"):
		return "A", pal.added
	case strings.HasPrefix(status, "D"):
		return "D", pal.deleted
	case strings.HasPrefix(status, "R"):
		return "R", pal.renamed
	}
	return "M", pal.modified
}

func splitPath(p string) (dir, name string) {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return "", p
	}
	return p[:i], p[i+1:]
}

func (a *App) drop(i int) {
	commits := a.snap.Commits
	a.confirm("Drop “"+commits[i].Name+"”?", "The commit is removed from the branch's history by an interactive rebase.", "Drop", func() {
		a.run("Dropping…", func() error { return a.repo.DropCommit(commits, i) })
	})
}

func (a *App) fixup(i int) {
	commits := a.snap.Commits
	a.run("Fixing up…", func() error { return a.repo.FixupCommit(commits, i) })
}

func (a *App) moveCommit(i, delta int) {
	commits := a.snap.Commits
	if j := i + delta; j < 0 || j >= len(commits) {
		return
	}
	a.run("Moving…", func() error { return a.repo.MoveCommit(commits, i, delta) }, func() { a.commitSel = i + delta })
}

func (a *App) reword(i int) {
	commits := a.snap.Commits
	cm := commits[i]
	repo := a.repo
	a.host.Go(func() {
		msg, err := repo.CommitMessage(cm)
		a.host.Post(func() {
			if err != nil {
				a.toastErr(err)
				return
			}
			a.prompt("Reword commit", strings.TrimSpace(msg), "Reword", true, func(text string) {
				summary, desc, _ := strings.Cut(strings.TrimSpace(text), "\n")
				a.run("Rewording…", func() error {
					return a.repo.RewordCommit(commits, i, strings.TrimSpace(summary), strings.TrimSpace(desc))
				})
			})
		})
	})
}
