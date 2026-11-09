// laziergit is lazygit's functionality in a native desktop app.
//
//	go tool mygo dev            # or: go run . [repository…]
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/Siemko/laziergit/internal/core"
)

func main() {
	// git runs this binary again as the sequence editor of lazygit's
	// interactive rebases.
	if core.HandleDaemon() {
		return
	}
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("laziergit", version())
		return
	}
	// Started from Finder or the Dock, the app has launchd's PATH, which
	// misses Homebrew's git, gpg, git-lfs and the tools hooks run.
	importLoginPath()
	debugf("PATH %s", os.Getenv("PATH"))

	capture := os.Getenv("LAZIERGIT_CAPTURE")
	// Started again, as from a terminal, the app gives its arguments to
	// the instance running, which opens their repositories.
	if capture == "" && !mygo.App.RequestSingleInstanceLock() {
		return
	}

	m := newManager(configDir())
	// args are the second instance's, without its program's name.
	mygo.App.OnSecondInstance(func(args []string, workingDir string) {
		debugf("second instance in %s: %q", workingDir, args)
		dirs := repoArgs(args, workingDir)
		if len(dirs) == 0 {
			m.focusOrNew()
		}
		for _, d := range dirs {
			m.open(d, nil)
		}
	})
	// A repository chosen in Open Recent, or dropped on the Dock icon.
	mygo.App.OnOpenFile(func(path string) { m.open(path, nil) })
	// The app stays with its Dock icon when its last window closes, as Mac
	// apps do, and opens one when the icon is clicked.
	mygo.App.OnWindowAllClosed(func() {
		if runtime.GOOS != "darwin" {
			mygo.App.Quit()
		}
	})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			m.newWindow("")
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { m.quitting() })

	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(m.menu())
		m.refreshRecent()
		dirs := repoArgs(os.Args[1:], "")
		if len(dirs) == 0 {
			if wd, err := os.Getwd(); err == nil && wd != "/" && isRepo(wd) {
				dirs = []string{wd} // run from a terminal in a repository
			} else {
				dirs = m.lastSession()
			}
		}
		for _, d := range dirs {
			m.open(d, nil)
		}
		if len(m.windows) == 0 {
			m.newWindow("")
		}
		if capture != "" {
			captureFirst(m, capture)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// repoArgs is the arguments that name directories, resolved against wd.
func repoArgs(args []string, wd string) []string {
	var out []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			continue
		}
		if !filepath.IsAbs(a) && wd != "" {
			a = filepath.Join(wd, a)
		}
		if st, err := os.Stat(a); err == nil && st.IsDir() {
			out = append(out, a)
		}
	}
	return out
}

// version is the app's: the bundle's, from mygo.json, or the module's for
// go install.
func version() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return strings.TrimPrefix(bi.Main.Version, "v")
	}
	return "dev"
}

func configDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "laziergit")
	}
	return ""
}

// captureFirst writes a PNG of the first window to path once it has
// loaded, then quits: for checking how the app looks from scripts.
// LAZIERGIT_CAPTURE_SECTION shows a section first.
func captureFirst(m *manager, path string) {
	w := m.windows[0]
	go func() {
		if s := os.Getenv("LAZIERGIT_CAPTURE_SECTION"); s != "" {
			time.Sleep(2 * time.Second)
			w.win.Update(func() { w.app.ShowSection(s) })
		}
		time.Sleep(3 * time.Second)
		png, err := w.win.CapturePage()
		if err != nil {
			log.Println("capture:", err)
		} else if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Println("capture:", err)
		}
		mygo.App.Quit()
	}()
}
