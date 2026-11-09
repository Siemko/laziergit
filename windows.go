package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/app"
	"github.com/Siemko/laziergit/internal/core"
)

// manager keeps the app's windows, each on a repository or, before one is
// opened, the welcome screen. Each window has its repository; a repository
// opened twice brings its window to the front.
type manager struct {
	configDir string
	windows   []*repoWindow
	// closing is set while the app quits, when windows closing one by one
	// don't change the session to restore.
	closing bool
	// recent is the repositories opened lately, newest first.
	recent []string
	// quitWhenIdle is set when a quit waits for the windows' actions.
	quitWhenIdle bool
}

// repoWindow is a window and the app in it.
type repoWindow struct {
	win       *mygo.Window
	app       *app.App
	stopWatch context.CancelFunc
	// opening is the repository the window is about to open, in the frame
	// it does: a window opening one isn't free for another.
	opening string
}

func newManager(configDir string) *manager { return &manager{configDir: configDir} }

// path is the repository the window has, or is opening.
func (w *repoWindow) path() string {
	if r := w.app.Repo(); r != nil {
		return r.Path
	}
	return w.opening
}

// open opens the repository dir is in: its window, if one shows it, comes
// to the front; otherwise from, a window without a repository, or a new
// window shows it.
func (m *manager) open(dir string, from *repoWindow) {
	// git finds the work tree off the UI thread, which a command of
	// another window can't then hold up.
	go func() {
		root := repoRoot(dir)
		mygo.RunOnMain(func() { m.openRoot(root, from) })
	}()
}

func (m *manager) openRoot(root string, from *repoWindow) {
	for _, w := range m.windows {
		if w.path() == root {
			debugf("window %d already has %s", w.win.ID(), root)
			w.win.Restore()
			w.win.Focus()
			return
		}
	}
	if from == nil || from.path() != "" {
		// A window without a repository yet, as the welcome window, takes it.
		for _, w := range m.windows {
			if w.path() == "" {
				from = w
				break
			}
		}
	}
	if from == nil || from.path() != "" {
		from = m.newWindow(root)
	}
	w := from
	w.opening = root
	w.app.Open(root, func(err error) {
		w.opening = ""
		if err != nil {
			log.Printf("opening %s: %v", root, err)
		}
	})
}

// newWindow opens a window, empty until a repository is opened in it. A
// window for a repository keeps where it was, by the repository's path.
func (m *manager) newWindow(path string) *repoWindow {
	w := &repoWindow{}
	w.app = app.New(app.Host{
		Post: func(fn func()) {
			if w.win != nil {
				w.win.Update(fn)
			}
		},
		Go: func(fn func()) { go fn() },
		OpenRepository: func() {
			if dir := chooseRepository(w.win); dir != "" {
				m.open(dir, w)
			}
		},
		Opened: func(r *core.Repo) {
			debugf("window %d opened %s", w.win.ID(), r.Path)
			w.win.SetTitle(r.Name())
			if err := mygo.App.AddRecentDocument(r.Path); err != nil {
				log.Println(err)
			}
			m.refreshRecent()
			m.saveSession()
		},
		Recent:     func() []string { return m.recent },
		Idle:       m.idle,
		OpenRecent: func(path string) { m.open(path, w) },
		Watch:      func(path string) { m.watch(w, path) },
	}, core.Options{ConfigDir: m.configDir})

	opts := mygo.WindowOptions{
		Title:          "laziergit",
		Width:          1320,
		Height:         860,
		MinWidth:       880,
		MinHeight:      520,
		StateKey:       "welcome",
		TitleBarStyle:  mygo.TitleBarHiddenInset,
		TitleBarHeight: 52,
		Vibrancy:       mygo.VibrancySidebar,
		Content:        ui.View(w.app.View),
	}
	if path != "" {
		opts.StateKey = "repo:" + path
	}
	// A window without a place of its own yet goes below and right of the
	// one in front, as Mac apps cascade them.
	if f := mygo.FocusedWindow(); f != nil {
		x, y := f.Position()
		opts.X, opts.Y = x+28, y+28
	}
	w.win = mygo.NewWindow(opts)
	debugf("window %d created (%d open)", w.win.ID(), len(m.windows)+1)
	w.win.OnClosed(func() {
		if w.stopWatch != nil {
			w.stopWatch()
		}
		w.app.Close()
		m.windows = slices.DeleteFunc(m.windows, func(x *repoWindow) bool { return x == w })
		m.saveSession()
	})
	m.windows = append(m.windows, w)
	return w
}

// focusOrNew brings a window to the front, or opens one.
func (m *manager) focusOrNew() {
	if len(m.windows) == 0 {
		m.newWindow("")
		return
	}
	w := m.windows[len(m.windows)-1]
	w.win.Restore()
	w.win.Focus()
}

// focused is the window in front, nil for none of the manager's.
func (m *manager) focused() *repoWindow {
	f := mygo.FocusedWindow()
	for _, w := range m.windows {
		if w.win == f {
			return w
		}
	}
	return nil
}

func (m *manager) menu() *mygo.Menu {
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "New Window", Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { m.newWindow("") }},
			{Label: "Open Repository…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) {
				from := m.focused()
				var parent *mygo.Window
				if from != nil {
					parent = from.win
				}
				if dir := chooseRepository(parent); dir != "" {
					m.open(dir, from)
				}
			}},
			{Role: mygo.RoleOpenRecent},
			mygo.Separator(),
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleToggleFullScreen},
		}},
		{Role: mygo.RoleWindowMenu},
	})
}

// chooseRepository asks for a repository's folder; empty when cancelled.
func chooseRepository(parent *mygo.Window) string {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: parent, Title: "Open Repository", ButtonLabel: "Open", Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return ""
	}
	return paths[0]
}

// refreshRecent reads the repositories opened lately: the system's recent
// documents of the app.
func (m *manager) refreshRecent() {
	m.recent = m.recent[:0]
	for _, p := range mygo.App.RecentDocuments() {
		if isRepo(p) {
			m.recent = append(m.recent, p)
		}
	}
}

// watch refreshes a window as its repository changes on disk, as when
// another program commits or edits files, in place of what it watched.
func (m *manager) watch(w *repoWindow, path string) {
	if w.stopWatch != nil {
		w.stopWatch()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.stopWatch = cancel
	go func() {
		fw, err := mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{
			Recursive: true,
			HintsOnly: true,
			Debounce:  250 * time.Millisecond,
			Ignore: func(p string, isDir bool) bool {
				switch p {
				case ".git/objects", ".git/logs", ".git/lfs", "node_modules":
					return true
				}
				return false
			},
		})
		if err != nil {
			log.Printf("watching %s: %v", path, err)
			return
		}
		defer fw.Close()
		for {
			if _, err := fw.Next(ctx); err != nil {
				return
			}
			w.win.Update(func() { w.app.ChangedOnDisk() })
		}
	}()
}

// The session is the repositories of the windows open, which the next
// launch opens again.

func (m *manager) sessionFile() string { return filepath.Join(m.configDir, "session.json") }

func (m *manager) saveSession() {
	if m.closing || m.configDir == "" {
		return
	}
	paths := []string{}
	for _, w := range m.windows {
		if p := w.path(); p != "" {
			paths = append(paths, p)
		}
	}
	data, _ := json.Marshal(paths)
	if err := writePrivate(m.sessionFile(), data); err != nil {
		log.Println(err)
	}
}

// beforeQuit holds a quit while a window's actions run, as an undo
// between its stash and its pop, and quits once they are done; then it
// keeps the session as it is, as the windows close. It holds the first
// quit alone: quitting again, as when the system logs out, goes through.
func (m *manager) beforeQuit(e *mygo.QuitEvent) {
	if !m.quitWhenIdle {
		for _, w := range m.windows {
			if w.app.Busy() {
				e.PreventDefault()
				m.quitWhenIdle = true
				debugf("quit waits for window %d", w.win.ID())
				for _, x := range m.windows {
					if x.app.Busy() {
						x.win.Update(func() { x.app.Notice("Quitting when git finishes", "Press ⌘Q again to quit now.") })
					}
				}
				return
			}
		}
	}
	m.quitWhenIdle = false
	m.saveSession()
	m.closing = true
}

// idle quits, when a quit waits, once no window has actions left.
func (m *manager) idle() {
	if !m.quitWhenIdle {
		return
	}
	for _, w := range m.windows {
		if w.app.Busy() {
			return
		}
	}
	mygo.App.Quit()
}

// writePrivate replaces a file the user alone reads, atomically.
func writePrivate(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path) // CreateTemp makes it 0600
}

func (m *manager) lastSession() []string {
	if m.configDir == "" {
		return nil
	}
	data, err := os.ReadFile(m.sessionFile())
	if err != nil {
		return nil
	}
	var paths []string
	_ = json.Unmarshal(data, &paths)
	return slices.DeleteFunc(paths, func(p string) bool { return !isRepo(p) })
}

func isRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// repoRoot is the work tree dir is in, dir itself when git can't say.
func repoRoot(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel")
	cmd.Env = withoutGitLocation(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}

// debugf logs what the windows do while $LAZIERGIT_DEBUG is set.
func debugf(format string, args ...any) {
	if os.Getenv("LAZIERGIT_DEBUG") != "" {
		log.Printf(format, args...)
	}
}

// withoutGitLocation is an environment without the variables that tell git
// where a repository is, which another window's repository sets in the
// process: git then finds it from its directory alone.
func withoutGitLocation(environ []string) []string {
	return slices.DeleteFunc(slices.Clone(environ), func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_NAMESPACE":
			return true
		}
		return false
	})
}
