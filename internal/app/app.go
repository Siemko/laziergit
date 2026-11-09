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

	"github.com/Siemko/laziergit/internal/core"
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
	// Idle, when set, is told when the window's actions are all done.
	Idle func()
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
	// accent is the window's, which lists without the keys show gray.
	accent ui.Color
	// listPref is the list's width as the user chose it, listShown the
	// one shown, held to the room there is, and bodyW the room, the frame
	// before.
	listPref, listShown, bodyW float32
	// detailW is the detail pane's width, the frame before: narrow, its
	// parts stack or fold.
	detailW float32
	// filesH is the height of a commit's files over its diff, stacked;
	// filesHShown the one shown, and filesHMoved whether the user moved it.
	filesH, filesHShown float32
	filesHMoved         bool
	// jobs are the actions asked for, the first running while running.
	jobs    []job
	running bool
	// credentials answer the credential prompts git waits on.
	credentials map[int]func(string)
	// closed is set once the window closed.
	closed bool
	// loadErr is why the first load failed; changedSince is set when the
	// repository changed on disk while the app was busy.
	loadErr      string
	changedSince bool
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
		filesH:   150,
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

// Open opens the repository dir is in, off the UI thread, and loads it.
// done, when set, is told how it went, on the UI thread.
func (a *App) Open(dir string, done func(error)) {
	opts := a.opts
	a.host.Go(func() {
		r, err := core.Open(dir, opts)
		a.host.Post(func() {
			if a.closed {
				// The window closed while the repository opened.
				if r != nil {
					a.host.Go(r.Close)
				}
				return
			}
			if err != nil {
				a.openErr = err.Error()
			} else {
				a.attach(r)
			}
			if done != nil {
				done(err)
			}
		})
	})
}

// attach shows a repository opened, in place of the one shown.
func (a *App) attach(r *core.Repo) {
	if old := a.repo; old != nil {
		a.host.Go(old.Close)
	}
	a.repo, a.snap, a.openErr, a.loadErr = r, nil, "", ""
	a.jobs = nil
	a.fileSel, a.commitSel, a.branchSel, a.stashSel = 0, 0, 0, 0
	a.detailKey, a.diffKey = "", ""
	a.refresh(false)
	if a.host.Watch != nil {
		a.host.Watch(r.Path)
	}
	if a.host.Opened != nil {
		a.host.Opened(r)
	}
}

// Close closes the repository, as the window closes: git waiting on a
// credential is answered, so that it stops, and the repository closes
// once its command ends, off the UI thread.
func (a *App) Close() {
	a.closed = true
	for _, answer := range a.credentials {
		answer("")
	}
	a.credentials = nil
	// Actions not started yet never start: only the one running, if any,
	// finishes.
	if a.running {
		a.busy -= len(a.jobs) - 1
		a.jobs = a.jobs[:1]
	} else {
		a.busy -= len(a.jobs)
		a.jobs = nil
	}
	if r := a.repo; r != nil {
		a.repo = nil
		r.Closing() // a command not started yet fails, whoever runs first
		a.host.Go(r.Close)
	}
}

// Notice shows a message in the window, as a toast.
func (a *App) Notice(title, desc string) {
	a.toasts = append(a.toasts, toastReq{title: title, desc: desc, kind: "info"})
}

// Busy reports whether an action runs or waits to.
func (a *App) Busy() bool { return a.busy > 0 }

// ChangedOnDisk refreshes after the work tree or the repository changed
// outside the app. It skips changes the app's own commands and refreshes
// just made, and leaves a refresh running to finish.
func (a *App) ChangedOnDisk() {
	if a.busy > 0 || a.loading {
		a.changedSince = true // refreshed once the work is done
		return
	}
	if time.Since(a.lastRefresh) < 400*time.Millisecond {
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
	a.changedSince = false
	a.host.Go(func() {
		s, err := repo.Load(background)
		a.host.Post(func() {
			a.loading = false
			a.lastRefresh = time.Now()
			if repo != a.repo {
				return
			}
			if err != nil {
				if a.snap == nil {
					a.loadErr = err.Error()
				} else {
					a.toastErr(err)
				}
				return
			}
			a.snap, a.loadErr = s, ""
			a.gen++
			a.clampSelections()
			if a.toConflicts {
				a.toConflicts = false
				a.showFirstConflict()
			}
			if a.changedSince && a.busy == 0 {
				a.refresh(true)
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

// job is an action of the window on its repository.
type job struct {
	repo  *core.Repo
	doing string
	fn    func(r *core.Repo) error
	after []func()
}

// run runs a git action on the window's repository off the UI thread,
// after those run before it, then refreshes. after runs, on the UI thread,
// when it succeeded. The action gets the repository as it was when it was
// asked for, whatever the window shows by the time it runs.
func (a *App) run(doing string, fn func(r *core.Repo) error, after ...func()) {
	if a.repo == nil || a.closed {
		return
	}
	a.busy++
	a.jobs = append(a.jobs, job{repo: a.repo, doing: doing, fn: fn, after: after})
	if !a.running {
		a.startJob()
	}
}

func (a *App) startJob() {
	j := a.jobs[0]
	a.running = true
	a.doing = j.doing
	a.host.Go(func() {
		err := j.fn(j.repo)
		a.host.Post(func() { a.endJob(j, err) })
	})
}

func (a *App) endJob(j job, err error) {
	a.busy--
	a.running = false
	if len(a.jobs) > 0 {
		a.jobs = a.jobs[1:]
	}
	switch {
	case a.closed || j.repo != a.repo:
	case core.IsConflict(err):
		a.toasts = append(a.toasts, toastReq{title: "Conflicts to resolve", desc: "Keep a side of each conflict in Changes, then continue.", kind: "warning"})
		a.addLog("conflicts: "+err.Error(), false)
		a.toConflicts = true
	case err != nil:
		a.toastErr(err)
	default:
		for _, f := range j.after {
			f()
		}
	}
	switch {
	case a.closed:
		a.jobs, a.doing = nil, ""
	case a.running:
		// An after callback asked for an action, which started.
	case len(a.jobs) > 0:
		a.startJob()
	default:
		a.doing = ""
		a.refresh(false)
	}
	if a.busy == 0 && a.host.Idle != nil {
		a.host.Idle()
	}
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
	a.accent = t.Accent
	for _, r := range a.toasts {
		timeout := 4 * time.Second
		if r.kind == "error" {
			timeout = 8 * time.Second
		}
		c.AddToast(ui.Toast{Title: r.title, Description: r.desc, Type: r.kind, Timeout: timeout})
	}
	a.toasts = nil
	toasts(c, pal)

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
		// On the window's material, the sidebar's rows are tinted, not
		// painted over it.
		side := *t
		if c.Vibrancy() {
			tint := ui.RGB(0, 0, 0)
			if t.Dark {
				tint = ui.RGB(255, 255, 255)
			}
			side.SurfaceHover, side.SurfacePressed = tint.Alpha(0.05), tint.Alpha(0.09)
			if t.Dark {
				side.SurfaceHover, side.SurfacePressed = tint.Alpha(0.06), tint.Alpha(0.10)
			}
		}
		ui.Themed(c, &side).Background(ui.Transparent).Width(a.sidebarW).Shrink(0).Children(func() { a.sidebar(c, pal) })
		ui.Box(c).Width(1).Background(pal.border)
		ui.Column(c).Grow(1).MinWidth(0).Background(pal.pane).Children(func() {
			a.toolbar(c, pal)
			a.stateBanner(c, pal)
			a.copiedBanner(c, pal)
			if a.snap == nil {
				ui.Column(c).Grow(1).Center().Gap(12).Padding(24).Children(func() {
					if a.loadErr == "" {
						ui.Spinner(c).Label("Loading")
						return
					}
					ui.Icon(c, iconWarning).FontSize(28).TextColor(pal.deleted)
					ui.Text(c, "The repository couldn't be read").FontSize(13).FontWeight(600)
					ui.Text(c, a.loadErr).FontSize(12).TextColor(pal.muted).MaxWidth(520).TextAlign(ui.Center).Selectable()
					if ui.PrimaryButton(c, "Retry").Clicked() {
						a.loadErr = ""
						a.refresh(false)
					}
				})
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
		// The list's width as the user left it, held to what leaves both
		// panes room as the window narrows; double-clicking the divider
		// resets it to a share of the window.
		if a.listW != a.listShown {
			a.listPref = a.listW // dragged
		}
		bodyW := a.bodyW
		reset := float32(380)
		if bodyW > 0 {
			reset = max(300, min(420, bodyW*0.42))
			if a.listPref <= 0 {
				a.listPref = reset
			}
			a.listW = max(280, min(a.listPref, bodyW-340))
		}
		a.listShown = a.listW
		split := ui.Split(c.Key("body"), &a.listW, func() {
			ui.Column(c).Fill().Children(func() { a.listPane(c, pal) })
		}, func() {
			detail := ui.Column(c.Key("detailPane")).Fill().Background(t(c).Background)
			a.detailW = detail.Bounds().W
			detail.Children(func() { a.detailPane(c, pal) })
		}).SplitReset(reset).Grow(1).MinHeight(0)
		a.bodyW = split.Bounds().W
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
	paneHeaderOf(c, pal, title, false, extra)
}

// pathHeader is a pane's bar titled with a file's path, shortened from its
// start to fit, so that the file's name shows.
func pathHeader(c *ui.Context, pal *palette, path string, extra func()) {
	paneHeaderOf(c, pal, path, true, extra)
}

func paneHeaderOf(c *ui.Context, pal *palette, title string, isPath bool, extra func()) {
	ui.Row(c).Height(headerHeight).Shrink(0).Padding(0, 10, 0, 14).Gap(8).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.border).Children(func() {
		// The title takes the room the rest leaves: its width, the frame
		// before, is what a path is fitted to.
		room := ui.Row(c.Key("title")).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Center)
		text := title
		if isPath {
			text = fitPath(c, title, room.Bounds().W, 12, 600)
			room.Tooltip(title)
		}
		room.Children(func() {
			ui.Text(c, text).FontSize(12).FontWeight(600).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
		})
		if extra != nil {
			extra()
		}
	})
}

func emptyState(c *ui.Context, pal *palette, svg *ui.SVG, title, hint string) {
	ui.Column(c).Grow(1).Center().Gap(8).Padding(24).Children(func() {
		ui.Icon(c, svg).FontSize(30).TextColor(pal.muted.Alpha(0.6))
		if title != "" {
			ui.Text(c, title).FontSize(13).FontWeight(600).TextAlign(ui.Center)
		}
		if hint != "" {
			ui.Text(c, hint).FontSize(12).TextColor(pal.muted).TextAlign(ui.Center).MaxWidth(300)
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
		ui.Column(c).Grow(1).MinHeight(0).Center().Gap(14).Padding(0, 16, titleBarHeight, 16).Children(func() {
			ui.Icon(c, iconBranch).FontSize(44).TextColor(tt.Accent)
			ui.Text(c, "laziergit").FontSize(22).FontWeight(700)
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
			// The recent repositories scroll in the room a small window has.
			_, h := c.Size()
			room := float32(264)
			if h < 640 {
				room = 168
			}
			ui.Column(c).Width(380).MaxWidthPercent(100).Margin(18, 0, 0, 0).Gap(2).Shrink(1).MinHeight(0).Children(func() {
				ui.Text(c, "Recent").FontSize(11).FontWeight(600).TextColor(pal.muted).Padding(0, 10, 4, 10)
				ui.Scroll(c.Key("recent")).MaxHeight(room).Children(func() {
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
	})
}

// ago is how long ago unix was, as a phrase: "just now", "5m ago".
func ago(unix int64) string {
	if r := relTime(unix); r != "now" {
		return r + " ago"
	}
	return "just now"
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
