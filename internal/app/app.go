// Package app is laziergit's window: lazygit's functionality, through
// package core, in MyGo's native UI.
package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jesseduffield/lazygit/pkg/commands/models"

	"laziergit/internal/core"
)

// section is what the sidebar shows in the main area.
type section string

const (
	secChanges  section = "changes"
	secHistory  section = "history"
	secBranches section = "branches"
	secStash    section = "stash"
)

var sectionOrder = []section{secChanges, secHistory, secBranches, secStash}

// pane is where the keyboard is, for FocusBind.
type pane int

const (
	paneNone pane = iota
	paneList
	paneFiles // a commit's or stash entry's files
	paneDiff
	paneSummary
	paneDescription
)

// Host is what the app needs of its window: to run a function on the UI
// thread and draw again, and to run work off it.
type Host struct {
	// Post runs fn on the UI thread and rebuilds the view.
	Post func(fn func())
	// Go runs fn off the UI thread.
	Go func(fn func())
	// OpenRepository asks for a repository to open, and opens it: in this
	// window while it has none, in a window of its own otherwise.
	OpenRepository func()
	// Opened is told of the repository the window opened.
	Opened func(r *core.Repo)
	// Recent lists the repositories opened lately, newest first, and
	// OpenRecent opens one, as OpenRepository does.
	Recent     func() []string
	OpenRecent func(path string)
	// Watch, when set, watches the work tree of a repository opened, to
	// call App.ChangedOnDisk on the UI thread, in place of the last one.
	Watch func(path string)
}

type logEntry struct {
	at   time.Time
	text string
	cmd  bool
}

// App is the state of a window on a repository.
type App struct {
	host Host
	opts core.Options

	repo    *core.Repo
	snap    *core.Snapshot
	openErr string
	loading bool
	gen     int // counts snapshots, so details know they are stale

	section section
	pane    pane
	busy    int
	doing   string

	sidebarW, listW, filesW float32

	// Lists and what they chose.
	filesList, commitsList, branchesList, stashList, detailFilesList ui.ListState
	fileSel, commitSel, branchSel, stashSel, detailFileSel           int

	// The detail of the row chosen.
	detailKey   string
	detailFiles []*models.CommitFile
	detailMsg   string
	diffKey     string
	diff        *editor.DiffState
	diffEmpty   string
	diffText    string
	diffStaged  int // 0 unstaged, 1 staged, for a file with both
	unified     int // 0 unified, 1 split
	stage       stagingView

	// The commits of the branch chosen in Branches.
	branchKey, branchName string
	branchCommits         []*models.Commit
	branchGraph           [][]core.Pipe
	branchCommitSel       int
	branchCommitsList     ui.ListState

	// Commits copied to cherry-pick, and the log they came from.
	copied     []copiedCommit
	copiedFrom string

	conflicts conflictView
	// offerContinue asks, once refreshed, to continue the operation
	// whose last conflict was resolved; toConflicts shows the first
	// conflicted file once a command stopped on conflicts.
	offerContinue, toConflicts bool
	lastRefresh                time.Time

	// The commit composer.
	summary, description string
	amend                bool

	toasts  []toastReq
	log     []logEntry
	logList ui.ListState
	logOpen bool

	dialog     *dialog
	palette    bool
	query      string
	paletteSel int
	paletteLst ui.ListState
}

// New makes the app; Open opens its repository.
func New(host Host, opts core.Options) *App {
	a := &App{
		host:     host,
		section:  secChanges,
		pane:     paneList,
		sidebarW: 210,
		listW:    380,
		filesW:   240,
	}
	opts.OnCommand = func(cmd string, commandLine bool) {
		a.host.Post(func() { a.addLog(cmd, commandLine) })
	}
	opts.AskCredential = func(kind core.Credential) <-chan string {
		ch := make(chan string, 1)
		a.host.Post(func() { a.askCredential(kind, ch) })
		return ch
	}
	a.opts = opts
	a.logList.FollowEnd = true
	return a
}

// Open opens the repository dir is in, and loads it.
func (a *App) Open(dir string) error {
	r, err := core.Open(dir, a.opts)
	if err != nil {
		a.openErr = err.Error()
		return err
	}
	if a.repo != nil {
		a.repo.Close()
	}
	a.repo, a.snap, a.openErr = r, nil, ""
	a.fileSel, a.commitSel, a.branchSel, a.stashSel = 0, 0, 0, 0
	a.detailKey, a.diffKey = "", ""
	a.refresh(false)
	if a.host.Watch != nil {
		a.host.Watch(r.Path)
	}
	if a.host.Opened != nil {
		a.host.Opened(r)
	}
	return nil
}

// Close closes the repository, as the window closes.
func (a *App) Close() {
	if a.repo != nil {
		a.repo.Close()
		a.repo = nil
	}
}

// ChangedOnDisk refreshes after the work tree or the repository changed
// outside the app. It skips changes the app's own commands and refreshes
// just made, and leaves a refresh running to finish.
func (a *App) ChangedOnDisk() {
	if a.busy > 0 || a.loading || time.Since(a.lastRefresh) < 400*time.Millisecond {
		return
	}
	a.refresh(true)
}

// ShowSection shows a section of the sidebar: changes, history, branches
// or stash.
func (a *App) ShowSection(name string) { a.goTo(section(name)) }

// Repo is the repository open, nil before one is.
func (a *App) Repo() *core.Repo { return a.repo }

func (a *App) addLog(text string, cmd bool) {
	a.log = append(a.log, logEntry{at: time.Now(), text: text, cmd: cmd})
	if len(a.log) > 500 {
		a.log = a.log[len(a.log)-500:]
	}
}

// refresh loads the repository again, off the UI thread.
func (a *App) refresh(background bool) {
	if a.repo == nil {
		return
	}
	repo := a.repo
	a.loading = true
	a.host.Go(func() {
		s, err := repo.Load(background)
		a.host.Post(func() {
			a.loading = false
			a.lastRefresh = time.Now()
			if repo != a.repo {
				return
			}
			if err != nil {
				a.toastErr(err)
				return
			}
			a.snap = s
			a.gen++
			a.clampSelections()
			if a.toConflicts {
				a.toConflicts = false
				a.showFirstConflict()
			}
		})
	})
}

// showFirstConflict shows the first file with conflicts in Changes.
func (a *App) showFirstConflict() {
	for i, f := range a.snap.Files {
		if f.HasMergeConflicts {
			a.section, a.fileSel, a.pane = secChanges, i, paneList
			a.filesList.ScrollIntoView(i)
			return
		}
	}
}

func (a *App) clampSelections() {
	clamp := func(sel *int, n int) {
		*sel = max(0, min(*sel, n-1))
	}
	clamp(&a.fileSel, len(a.snap.Files))
	clamp(&a.commitSel, len(a.snap.Commits))
	clamp(&a.branchSel, len(a.snap.Branches))
	clamp(&a.stashSel, len(a.snap.Stash))
}

// run runs a git action off the UI thread, then refreshes.
func (a *App) run(doing string, fn func() error, after ...func()) {
	if a.repo == nil {
		return
	}
	a.busy++
	a.doing = doing
	a.host.Go(func() {
		err := fn()
		a.host.Post(func() {
			a.busy--
			if a.busy == 0 {
				a.doing = ""
			}
			if core.IsConflict(err) {
				a.toasts = append(a.toasts, toastReq{title: "Conflicts to resolve", desc: "Keep a side of each conflict in Changes, then continue.", kind: "warning"})
				a.addLog("conflicts: "+err.Error(), false)
				a.toConflicts = true
			} else if err != nil {
				a.toastErr(err)
			} else {
				for _, f := range after {
					f()
				}
			}
			a.refresh(false)
		})
	})
}

type toastReq struct {
	title, desc, kind string
}

func (a *App) toastErr(err error) {
	a.toasts = append(a.toasts, toastReq{title: "Git failed", desc: err.Error(), kind: "error"})
	a.addLog("error: "+err.Error(), false)
}

func (a *App) toast(title, desc string) {
	a.toasts = append(a.toasts, toastReq{title: title, desc: desc, kind: "success"})
}

// View builds the window.
func (a *App) View(c *ui.Context) {
	t := c.Theme()
	pal := paletteFor(t)
	for _, r := range a.toasts {
		timeout := 4 * time.Second
		if r.kind == "error" {
			timeout = 8 * time.Second
		}
		c.AddToast(ui.Toast{Title: r.title, Description: r.desc, Type: r.kind, Timeout: timeout})
	}
	a.toasts = nil

	if c.Vibrancy() {
		c.Root().Background(ui.Transparent)
	} else {
		c.Root().Background(t.Surface)
	}
	if a.repo == nil {
		a.welcome(c, pal)
		return
	}
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(a.sidebarW).Shrink(0).Children(func() { a.sidebar(c, pal) })
		ui.Box(c).Width(1).Background(pal.border)
		ui.Column(c).Grow(1).MinWidth(0).Background(pal.pane).Children(func() {
			a.toolbar(c, pal)
			a.stateBanner(c, pal)
			a.copiedBanner(c, pal)
			if a.snap == nil {
				ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c).Label("Loading") })
				return
			}
			a.main(c, pal)
			a.statusBar(c, pal)
		})
	})
	a.offerToContinue()
	// Some pane always has the keys, as in lazygit: when the focus went
	// nowhere, as with the element a dialog gave it back to gone, the
	// list takes it.
	if a.pane == paneNone && a.dialog == nil && !a.palette && !c.FocusTakesText() {
		a.pane = paneList
	}
	a.shortcuts(c)
	a.dialogs(c, pal)
	a.paletteView(c, pal)
}

func (a *App) main(c *ui.Context, pal *palette) {
	body := func() {
		ui.Split(c.Key("body"), &a.listW, func() {
			ui.Column(c).Fill().Children(func() { a.listPane(c, pal) })
		}, func() {
			ui.Column(c).Fill().Background(t(c).Background).Children(func() { a.detailPane(c, pal) })
		}).SplitReset(380).Grow(1).MinHeight(0)
	}
	if !a.logOpen {
		body()
		return
	}
	ui.Column(c.Key("bodyAndLog")).Grow(1).MinHeight(0).Children(func() {
		body()
		ui.Box(c).Height(1).Background(pal.border)
		a.logView(c, pal)
	})
}

func t(c *ui.Context) *ui.Theme { return c.Theme() }

func (a *App) listPane(c *ui.Context, pal *palette) {
	switch a.section {
	case secChanges:
		a.changesList(c, pal)
	case secHistory:
		a.historyList(c, pal)
	case secBranches:
		a.branchList(c, pal)
	case secStash:
		a.stashListView(c, pal)
	}
}

func (a *App) detailPane(c *ui.Context, pal *palette) {
	switch a.section {
	case secChanges:
		a.fileDetail(c, pal)
	case secHistory:
		a.commitDetail(c, pal)
	case secBranches:
		a.branchDetail(c, pal)
	case secStash:
		a.stashDetail(c, pal)
	}
}

// paneHeader is the bar over a list or a detail.
func paneHeader(c *ui.Context, pal *palette, title string, extra func()) {
	ui.Row(c).Height(34).Shrink(0).Padding(0, 10, 0, 14).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if extra != nil {
			extra()
		}
	})
}

func emptyState(c *ui.Context, pal *palette, svg *ui.SVG, title, hint string) {
	ui.Column(c).Grow(1).Center().Gap(8).Padding(24).Children(func() {
		ui.Icon(c, svg).FontSize(28).TextColor(pal.muted)
		ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(pal.muted)
		if hint != "" {
			ui.Text(c, hint).FontSize(12).TextColor(pal.muted).TextAlign(ui.Center)
		}
	})
}

func (a *App) welcome(c *ui.Context, pal *palette) {
	tt := c.Theme()
	var recent []string
	if a.host.Recent != nil {
		recent = a.host.Recent()
	}
	ui.Column(c).Fill().Background(tt.Background).Children(func() {
		ui.Box(c).Height(titleBarHeight).Shrink(0).DragWindow()
		ui.Column(c).Grow(1).Center().Gap(14).Padding(0, 0, titleBarHeight, 0).Children(func() {
			ui.Icon(c, iconBranch).FontSize(44).TextColor(tt.Accent)
			ui.Text(c, "laziergit").FontSize(24).Bold()
			ui.Text(c, "lazygit's power, as a native app.").TextColor(pal.muted)
			if a.openErr != "" {
				ui.Text(c, a.openErr).FontSize(12).TextColor(pal.deleted).MaxWidth(420).TextAlign(ui.Center)
			}
			if ui.PrimaryButton(c, "Open Repository…").Margin(8, 0, 0, 0).Tooltip("⌘O").Clicked() {
				a.openFolder()
			}
			if len(recent) == 0 {
				return
			}
			ui.Column(c).Width(380).Margin(18, 0, 0, 0).Gap(2).Children(func() {
				ui.Text(c, "Recent").FontSize(11).FontWeight(600).TextColor(pal.muted).Padding(0, 10, 4, 10)
				for _, path := range recent[:min(len(recent), 8)] {
					b := ui.ButtonBase(c.Key(path)).Padding(7, 10).Gap(10).Radius(7).AlignItems(ui.Center).Label(path)
					if b.Hovered() {
						b.Background(pal.hover)
					}
					b.Children(func() {
						ui.Icon(c, iconFolder).FontSize(15).TextColor(tt.Accent)
						ui.Column(c).Grow(1).MinWidth(0).Children(func() {
							ui.Text(c, filepath.Base(path)).FontSize(13).FontWeight(500).SingleLine()
							ui.Text(c, abbreviateHome(filepath.Dir(path))).FontSize(11).TextColor(pal.muted).SingleLine()
						})
					})
					if b.Clicked() && a.host.OpenRecent != nil {
						a.host.OpenRecent(path)
					}
				}
			})
		})
	})
}

func relTime(unix int64) string {
	d := time.Since(time.Unix(unix, 0))
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/24/30))
	}
	return fmt.Sprintf("%dy", int(d.Hours()/24/365))
}
