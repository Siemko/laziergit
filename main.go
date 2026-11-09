// laziergit is lazygit's functionality in a native desktop app.
//
//	go tool mygo dev            # or: go run . [repository]
package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"laziergit/internal/app"
	"laziergit/internal/core"
)

func main() {
	// git runs this binary again as the sequence editor of lazygit's
	// interactive rebases.
	if core.HandleDaemon() {
		return
	}

	dir := ""
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		dir = os.Args[1]
	} else if wd, err := os.Getwd(); err == nil && wd != "/" {
		dir = wd // run from a terminal; an app started from Finder has "/"
	}

	var win *mygo.Window
	var a *app.App
	var stopWatch context.CancelFunc = func() {}
	a = app.New(app.Host{
		Post: func(fn func()) {
			if win != nil {
				win.Update(fn)
			}
		},
		Go: func(fn func()) { go fn() },
		OpenFolder: func() string {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: win, Title: "Open Repository", ButtonLabel: "Open", Directory: true,
			})
			if err != nil || len(paths) == 0 {
				return ""
			}
			return paths[0]
		},
		// Watch refreshes the window as the repository changes on disk, as
		// when another program commits or edits files.
		Watch: func(path string) {
			stopWatch()
			var ctx context.Context
			ctx, stopWatch = context.WithCancel(context.Background())
			go watchRepo(ctx, win, a, path)
		},
	}, core.Options{ConfigDir: configDir()})

	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:          "laziergit",
			Width:          1320,
			Height:         860,
			MinWidth:       880,
			MinHeight:      520,
			StateKey:       "main",
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: 52,
			Vibrancy:       mygo.VibrancySidebar,
			Content:        ui.View(a.View),
		})
		if dir != "" {
			win.Update(func() {
				_ = a.Open(dir)
			})
		}
		captureIfAsked(win, a)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func configDir() string {
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "laziergit")
	}
	return ""
}

func watchRepo(ctx context.Context, win *mygo.Window, a *app.App, path string) {
	w, err := mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{
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
	defer w.Close()
	for {
		if _, err := w.Next(ctx); err != nil {
			return
		}
		win.Update(func() { a.ChangedOnDisk() })
	}
}

// captureIfAsked writes a PNG of the window to $LAZIERGIT_CAPTURE once it
// has loaded, then quits: for checking how the app looks from scripts.
func captureIfAsked(win *mygo.Window, a *app.App) {
	path := os.Getenv("LAZIERGIT_CAPTURE")
	if path == "" {
		return
	}
	go func() {
		if s := os.Getenv("LAZIERGIT_CAPTURE_SECTION"); s != "" {
			time.Sleep(2 * time.Second)
			win.Update(func() { a.ShowSection(s) })
		}
		time.Sleep(3 * time.Second)
		png, err := win.CapturePage()
		if err != nil {
			log.Println("capture:", err)
		} else if err := os.WriteFile(path, png, 0o644); err != nil {
			log.Println("capture:", err)
		}
		mygo.App.Quit()
	}()
}
