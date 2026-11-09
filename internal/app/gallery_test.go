package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/core"
)

// TestGallery saves a picture of every view and dialog of the app, in a
// repository made to strain the layout (long names, many files, a
// conflict), at the default and the smallest window size, in the light
// and the dark, into $LAZIERGIT_GALLERY. It is for looking at the design:
//
//	LAZIERGIT_GALLERY=/tmp/gallery go test -run TestGallery ./internal/app
func TestGallery(t *testing.T) {
	out := os.Getenv("LAZIERGIT_GALLERY")
	if out == "" {
		t.Skip("LAZIERGIT_GALLERY not set")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	sizes := []struct {
		name string
		w, h int
	}{{"default", 1320, 860}, {"small", 880, 520}}
	for _, size := range sizes {
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			t.Run(size.name+"-"+theme, func(t *testing.T) {
				g := &gallery{t: t, dir: out, suffix: size.name + "-" + theme}
				g.views(size.w, size.h, dark)
			})
		}
	}
}

type gallery struct {
	t      *testing.T
	dir    string
	suffix string
	tt     *ui.Tester
	dark   bool
	n      int
}

func (g *gallery) shot(name string) {
	g.t.Helper()
	g.tt.Frame()
	g.tt.Advance(time.Second) // toasts and sheets in place
	g.tt.Frame()
	g.n++
	f, err := os.Create(filepath.Join(g.dir, fmt.Sprintf("%02d-%s-%s.png", g.n, name, g.suffix)))
	if err != nil {
		g.t.Fatal(err)
	}
	defer f.Close()
	// The tester leaves the window's material (the sidebar's) clear: it is
	// painted here about as macOS does.
	src := g.tt.Image()
	img := image.NewRGBA(src.Bounds())
	material := color.RGBA{236, 236, 236, 255}
	if g.dark {
		material = color.RGBA{44, 44, 46, 255}
	}
	draw.Draw(img, img.Bounds(), image.NewUniform(material), image.Point{}, draw.Src)
	draw.Draw(img, img.Bounds(), src, src.Bounds().Min, draw.Over)
	if err := png.Encode(f, img); err != nil {
		g.t.Fatal(err)
	}
}

func (g *gallery) start(view func(*ui.Context), w, h int, dark bool) {
	g.tt = ui.NewTester(view, w, h)
	g.tt.SetScale(2)
	g.tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Left: 78})
	g.tt.SetVibrancy(true)
	g.tt.SetDark(dark)
	g.dark = dark
	g.tt.Frame()
}

func (g *gallery) views(w, h int, dark bool) {
	t := g.t
	// The welcome window, with recent repositories of long paths.
	welcome := New(Host{
		Post: func(fn func()) { fn() }, Go: func(fn func()) { fn() },
		Recent: func() []string {
			return []string{
				"/Users/ada/code/engine",
				"/Users/ada/Developer/clients/northwind-traders/monorepo-with-a-rather-long-name",
				"/Users/ada/code/notes",
				"/Users/ada/code/website",
				"/Users/ada/code/dotfiles",
				"/Users/ada/Developer/oss/mygo",
				"/Users/ada/Developer/oss/lazygit",
				"/Users/ada/code/scratch",
			}
		},
	}, core.Options{})
	g.start(welcome.View, w, h, dark)
	g.shot("welcome")

	dir := messyFixture(t)
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	a := New(Host{Post: func(fn func()) { fn() }, Go: func(fn func()) { fn() }},
		core.Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	a.Open(dir, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	defer a.Close()
	g.start(a.View, w, h, dark)
	tt := g.tt

	// Changes: the list, a file's diff, its lines to stage, the composer.
	g.shot("changes")
	for i, f := range a.snap.Files {
		if strings.HasSuffix(f.Path, "PerformanceMetricsChartWithAnnotations.tsx") {
			a.fileSel = i
		}
	}
	g.shot("changes-long-file")
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Shift, ui.KeyDown)
	g.shot("staging")
	tt.Key(0, ui.KeyEscape)
	a.pane = paneNone
	tt.Key(0, ui.KeyC)
	tt.Type("Make every widget measure itself before the layout runs, so that resizing no longer flickers\n\nThe chart measured its labels after layout, which moved them a frame late.")
	g.shot("composer")
	tt.Key(0, ui.KeyEscape)
	a.pane = paneNone

	// History, with a commit's details.
	a.goTo(secHistory)
	g.shot("history")
	a.commitSel = 3
	g.shot("history-merge")

	// Branches and a branch's commits, then the stash.
	a.goTo(secBranches)
	for i, b := range a.snap.Branches {
		if strings.HasPrefix(b.Name, "feature/JIRA") {
			a.branchSel = i
		}
	}
	g.shot("branches")
	a.goTo(secStash)
	g.shot("stash")

	// The palette, and each kind of dialog.
	a.goTo(secChanges)
	tt.Key(ui.Cmd, ui.KeyK)
	tt.Type("sta")
	g.shot("palette")
	tt.Type("zzz")
	g.shot("palette-empty")
	tt.Key(0, ui.KeyEscape)
	a.palette = false

	a.confirmDiscard(a.selectedFile())
	g.shot("confirm")
	a.dialog = nil
	a.prompt("New branch from main", "feature/", "Create", false, func(string) {})
	g.shot("prompt")
	a.dialog = nil
	answer := a.opts.AskCredential(0)
	g.shot("credential")
	a.dialog.onCancel()
	<-answer
	a.dialog = nil

	// A long branch checked out, and the list dragged narrow.
	git(t, dir, "checkout", "-q", "-b", "feature/JIRA-67890-rework-the-export-dialog-column-picker-and-csv-options")
	a.refresh(false)
	a.listW = 280
	g.shot("long-branch-narrow-list")
	a.listW = 380

	a.toast("Pushed", "main to origin/main")
	a.toastErr(fmt.Errorf("git push: ! [rejected] main -> main (fetch first)\nerror: failed to push some refs to 'github.com:ada/engine.git'\nhint: Updates were rejected because the remote contains work that you do not have locally."))
	g.shot("toasts")
	a.Close()

	// A merge stopped on conflicts.
	c, _, _ := testerFor(t, conflictFixture(t))
	g.start(c.View, w, h, dark)
	c.goTo(secBranches)
	for i, b := range c.snap.Branches {
		if b.Name == "feature" {
			c.branchSel = i
		}
	}
	g.tt.Frame()
	g.tt.Key(ui.Shift, ui.KeyM)
	g.tt.Frame()
	g.tt.Key(0, ui.KeyEnter)
	g.shot("conflicts")
	c.Close()
}

// messyFixture makes a repository to strain the layout: long paths and
// names, many files of every status, long commit subjects, merges, many
// branches, one ahead of and behind its remote, and stashes.
func messyFixture(t *testing.T) string {
	root := t.TempDir()
	dir := filepath.Join(root, "northwind-traders-monorepo")
	remote := filepath.Join(root, "remote.git")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, ".gitconfig-test"))
	git(t, root, "init", "-q", "--bare", remote)
	git(t, root, "init", "-q", "-b", "main", dir)
	git(t, dir, "config", "user.name", "Bartholomew Fitzgerald-Montgomery")
	git(t, dir, "config", "user.email", "bartholomew.fitzgerald-montgomery@northwind-traders.example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	commit := func(msg string, files ...string) {
		for i := 0; i+1 < len(files); i += 2 {
			write(t, dir, files[i], files[i+1])
		}
		git(t, dir, "add", "-A")
		git(t, dir, "commit", "-q", "-m", msg)
	}
	chart := "src/components/dashboard/widgets/PerformanceMetricsChartWithAnnotations.tsx"
	long := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "export const series%d = computeSeries({ window: %d, smoothing: 'exponential', annotations: loadAnnotationsForTheDashboard(%d) }) // line %d\n", i, i*7, i, i)
		}
		return b.String()
	}
	commit("Initial commit", "README.md", "# Northwind\n", chart, long(40), "app/[slug]/page.tsx", "export default function Page() {}\n")
	commit("Add the dashboard", "src/dashboard.ts", "export {}\n")
	for i := 0; i < 12; i++ {
		commit(fmt.Sprintf("Tune the cache eviction policy, part %d", i+1), fmt.Sprintf("src/cache/policy%d.ts", i), "export {}\n")
	}
	git(t, dir, "checkout", "-q", "-b", "feature/JIRA-12345-measure-widgets-before-layout-to-stop-the-flicker")
	commit("Refactor the rendering pipeline so that every widget measures itself before layout, which fixes the flicker on resize",
		"src/render.ts", "export const measure = true\n")
	commit("Measure the chart's labels", "src/labels.ts", "export {}\n")
	git(t, dir, "checkout", "-q", "main")
	commit("Write the architecture overview", "docs/Überblick über die Architektur.md", "# Überblick\n")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge branch 'feature/JIRA-12345-measure-widgets-before-layout-to-stop-the-flicker'",
		"feature/JIRA-12345-measure-widgets-before-layout-to-stop-the-flicker")
	git(t, dir, "tag", "v2.14.0-rc.1")
	for _, b := range []string{"release/2026-11", "fix/typo", "experiment/wasm-renderer", "dependabot/npm_and_yarn/typescript-5.9.3"} {
		git(t, dir, "branch", b)
	}
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "push", "-q", "-u", "origin", "main")
	// Behind by one, ahead by two.
	commit("Publish the November release notes", "CHANGELOG.md", "## 2.14\n")
	git(t, dir, "push", "-q", "origin", "main")
	git(t, dir, "reset", "-q", "--hard", "HEAD~1")
	git(t, dir, "fetch", "-q", "origin")
	commit("Fix the tooltip's position near the window's edge", "src/tooltip.ts", "export {}\n")
	commit("Bump the chart library", "package.json", "{}\n")

	for i := 0; i < 3; i++ {
		write(t, dir, "src/wip.ts", fmt.Sprintf("// %d\n", i))
		git(t, dir, "add", "src/wip.ts")
		git(t, dir, "stash", "push", "-q", "-m", fmt.Sprintf("WIP on the export dialog: half-done column picker and the CSV options, take %d", i+1))
	}

	// Every status, and enough files to scroll.
	write(t, dir, chart, strings.Replace(long(40), "line 3\n", "line 3 — now with a much longer comment that runs well past the edge of the diff pane\n", 1)+"export const added = true\n")
	write(t, dir, "README.md", "# Northwind\n\nThe monorepo.\n")
	git(t, dir, "add", "README.md")
	os.MkdirAll(filepath.Join(dir, "src", "dashboard"), 0o755)
	git(t, dir, "mv", "src/dashboard.ts", "src/dashboard/index.ts")
	os.Remove(filepath.Join(dir, "package.json"))
	write(t, dir, "app/[slug]/page.tsx", "export default function Page() { return null }\n")
	write(t, dir, "docs/Überblick über die Architektur.md", "# Überblick\n\nNeu.\n")
	for i := 0; i < 18; i++ {
		write(t, dir, fmt.Sprintf("src/generated/locales/translations-for-region-%02d.json", i), "{}\n")
	}
	return dir
}
