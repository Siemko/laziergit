package app

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestMedia makes the README's pictures, the social preview and the
// frames of the demo, from a copy of laziergit's own repository, into
// $LAZIERGIT_MEDIA; scripts/media.sh makes the demo's video and GIF of the
// frames:
//
//	LAZIERGIT_MEDIA=build/media go test -run TestMedia ./internal/app
func TestMedia(t *testing.T) {
	out := os.Getenv("LAZIERGIT_MEDIA")
	if out == "" {
		t.Skip("LAZIERGIT_MEDIA not set")
	}
	if !filepath.IsAbs(out) {
		out = filepath.Join("..", "..", out) // from the repository's root
	}
	// Opening a repository changes the working directory.
	out, err := filepath.Abs(out)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(out, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := &media{t: t, out: out, root: root}
	m.stills()
	m.demo()
}

const (
	mediaScale      = 2 // a Retina display's
	mediaW, mediaH  = 1320, 820
	videoW, videoH  = 3200, 1800 // 16:9, about the window
	stillMargin     = 96         // room for the shadow, at 1×
	captionFontSize = 18
	frameRate       = 30
	frameDuration   = time.Second / frameRate
)

type media struct {
	t   *testing.T
	out string
	// root is the repository the tests run in.
	root string
}

// repo is a copy of laziergit's repository, as one in use: in ~/Developer
// of a home of its own, a release branch to back commits up to, and
// changes to stage. It is cloned from the repository the tests run in.
func (m *media) repo(readme bool) string {
	t := m.t
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home) // the sidebar says ~/Developer
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig"))
	dir := filepath.Join(home, "Developer", "laziergit")
	git(t, home, "clone", "-q", m.root, dir)
	git(t, dir, "config", "user.name", "Dominik Guzy")
	git(t, dir, "config", "user.email", "dominik@guzy.dev")
	git(t, dir, "config", "commit.gpgsign", "false")
	git(t, dir, "remote", "set-url", "origin", "https://github.com/Siemko/laziergit.git")
	git(t, dir, "branch", "release-0.5", "v0.5.0")
	// Work in progress: a line of the README and a function changed.
	edit := func(name, old, new string) {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, []byte(old)) {
			t.Fatalf("%s has no %q", name, old)
		}
		if err := os.WriteFile(path, bytes.Replace(data, []byte(old), []byte(new), 1), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	edit("internal/app/util.go", "\tif sel {\n\t\treturn tt.AccentText.Alpha(0.92)\n\t}",
		"\tif sel {\n\t\t// The accent's text, a touch softer than the title's.\n\t\treturn tt.AccentText.Alpha(0.88)\n\t}")
	edit("internal/app/util.go", "\tif n == 1 {\n\t\treturn fmt.Sprintf(\"1 %s\", noun)\n\t}",
		"\tif n == 1 {\n\t\treturn \"one \" + noun\n\t}")
	if readme {
		// Not in the demo's: the release branch's README is another, which
		// a checkout of it would overwrite.
		edit("README.md", "A proof of concept: ", "A native Mac app: ")
	}
	return dir
}

// window opens the repository in a window of the media's size and scale.
func (m *media) window(dir string, dark bool) (*App, *ui.Tester) {
	a, tt, _ := testerFor(m.t, dir)
	tt.SetSize(mediaW, mediaH)
	tt.SetScale(mediaScale)
	// As macOS shows it: room for the window's buttons, the sidebar on the
	// window's material (which the stage paints).
	tt.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Left: 78})
	tt.SetVibrancy(true)
	tt.SetDark(dark)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	return a, tt
}

// save writes img as a PNG in the media's directory.
func (m *media) save(name string, img image.Image) {
	m.t.Helper()
	f, err := os.Create(filepath.Join(m.out, name))
	if err != nil {
		m.t.Fatal(err)
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if strings.HasPrefix(name, "frames/") {
		enc.CompressionLevel = png.BestSpeed
	}
	if err := enc.Encode(f, img); err != nil {
		m.t.Fatal(err)
	}
}

// still is the window's content framed, on a clear canvas, for the README.
func still(win image.Image, dark bool) *image.RGBA {
	w, h := win.Bounds().Dx(), win.Bounds().Dy()
	pad := stillMargin * mediaScale
	st := newStage(w+2*pad, h+2*pad, nil, pad, pad-pad/4, w, h, mediaScale, dark)
	return st.compose(win)
}

// center is the middle of the element showing text s, in DIPs.
func (m *media) center(tt *ui.Tester, s string) (float32, float32) {
	m.t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		m.t.Fatalf("no %q in %q", s, tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// stills are the README's pictures: History with a range of commits
// chosen, in the light and the dark; line staging; the Settings window.
func (m *media) stills() {
	dir := m.repo(true)
	for _, dark := range []bool{false, true} {
		a, tt := m.window(dir, dark)
		tt.Key(0, ui.Key2)
		tt.Frame()
		m.drag(tt, "Number the sidebar's sections top to bottom, and center their keys",
			"Share the commits copied to cherry-pick among a repository's worktrees")
		// Code in the diff, the keys back on the range, which shows in the
		// accent.
		x, y := m.center(tt, "history_ops.go")
		tt.ClickAt(x, y)
		tt.Frame()
		a.pane = paneList
		m.rest(tt)
		name := "history"
		if dark {
			name += "-dark"
		}
		m.save(name+".png", still(tt.Image(), dark))
		if !dark {
			m.save("social.png", m.social(tt.Image()))
		}
	}
	// Line staging: the function's lines chosen in the diff.
	a, tt := m.window(dir, false)
	x, y := m.center(tt, "util.go")
	tt.ClickAt(x, y)
	tt.Frame()
	a.pane = paneDiff
	tt.Frame()
	tt.Key(0, ui.KeyJ)
	tt.Key(ui.Shift, ui.KeyJ)
	tt.Frame()
	m.rest(tt)
	m.save("staging.png", still(tt.Image(), false))
	// Settings.
	s := DefaultSettings()
	p := &SettingsPane{S: &s}
	st := ui.NewTester(p.View, 580, 460)
	st.SetScale(mediaScale)
	st.SetTitleBar(ui.TitleBar{Height: titleBarHeight, Left: 78})
	st.Frame()
	m.save("settings.png", still(st.Image(), false))
}

// drag presses on the row showing from and lets go on the one showing to.
func (m *media) drag(tt *ui.Tester, from, to string) {
	x, y := m.center(tt, from)
	_, y2 := m.center(tt, to)
	tt.Press(x, y)
	tt.Frame()
	for i := 1; i <= 6; i++ {
		tt.Move(x, y+(y2-y)*float32(i)/6)
		tt.Frame()
	}
	tt.Release(x, y2)
	tt.Frame()
	m.rest(tt)
}

// rest moves the pointer off the window, for no row to show it hovered nor
// a tooltip, and lets what moves settle.
func (m *media) rest(tt *ui.Tester) {
	tt.Move(-40, -40)
	tt.Frame()
	tt.Advance(2 * time.Second)
	tt.Frame()
}

// social is the repository's social preview, 1280×640: the app's name and
// what it is, beside a picture of it.
func (m *media) social(win image.Image) image.Image {
	const w, h = 1280, 640
	bg := gradient(w*mediaScale, h*mediaScale, rgba(24, 28, 48, 255), rgba(56, 36, 96, 255))
	// The window, large, running off the right and bottom edges.
	shot := image.NewRGBA(bg.Bounds())
	ww, wh := win.Bounds().Dx(), win.Bounds().Dy()
	st := newStage(w*mediaScale, h*mediaScale, bg, 600*mediaScale, 150*mediaScale, ww, wh, mediaScale, false)
	draw.Draw(shot, shot.Bounds(), st.compose(win), image.Point{}, draw.Src)
	// The words, drawn by the app's own text, over it.
	icon, err := os.ReadFile(filepath.Join(m.root, "resources", "icon.png"))
	if err != nil {
		m.t.Fatal(err)
	}
	bitmap, err := ui.DecodeBitmap(icon)
	if err != nil {
		m.t.Fatal(err)
	}
	text := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Column(c).Fill().Padding(84, 64).Gap(18).Children(func() {
			ui.Image(c, bitmap).Size(112, 112)
			ui.Text(c, "laziergit").FontSize(64).FontWeight(800).TextColor(ui.RGB(255, 255, 255))
			ui.Text(c, "lazygit's power,\nas a native Mac app.").FontSize(30).FontWeight(500).LineHeight(1.25).
				TextColor(ui.RGBA(255, 255, 255, 0.82)).MaxWidth(470)
			ui.Text(c, "Drawn on the GPU. No Electron, no webview.").FontSize(19).TextColor(ui.RGBA(255, 255, 255, 0.6)).
				Margin(10, 0, 0, 0)
		})
	}, w, h)
	text.SetScale(mediaScale)
	text.Frame()
	draw.Draw(shot, shot.Bounds(), text.Image(), image.Point{}, draw.Over)
	return shot
}

// demo records the demo's frames: a range of commits chosen with the
// mouse, copied, and cherry-picked onto a release branch; lines staged.
func (m *media) demo() {
	dir := m.repo(false)
	a, tt := m.window(dir, false)
	r := m.recorder(a, tt)
	r.say("")
	r.hold(800 * time.Millisecond)

	r.key(0, ui.Key2, "History")
	r.say("Drag over commits to choose them")
	x, y := m.center(tt, "Share the commits copied to cherry-pick among a repository's worktrees")
	_, y2 := m.center(tt, "Number the sidebar's sections top to bottom, and center their keys")
	r.moveTo(x-60, y, 700*time.Millisecond)
	r.dragTo(x-60, y2, 900*time.Millisecond)
	r.hold(900 * time.Millisecond)

	r.say("Copy them to cherry-pick")
	r.clickOn("Copy")
	r.hold(1200 * time.Millisecond)

	r.say("Backport them: check out the release branch")
	r.key(0, ui.Key3, "Branches")
	r.clickOn("release-0.5")
	r.hold(500 * time.Millisecond)
	r.clickOn("Checkout")
	r.hold(1200 * time.Millisecond)

	r.say("Paste them onto it: ⇧V")
	r.key(ui.Shift, ui.KeyV, "")
	r.hold(700 * time.Millisecond)
	r.clickOn("Cherry-Pick")
	r.hold(1800 * time.Millisecond)

	r.say("Stage single lines, not whole files")
	r.key(0, ui.Key1, "Changes")
	r.clickOn("util.go")
	r.hold(600 * time.Millisecond)
	r.say("Pick a line with j, stage it with space")
	r.a.pane = paneDiff
	r.key(0, ui.KeyJ, "")
	r.hold(700 * time.Millisecond)
	r.key(0, ui.KeySpace, "")
	r.hold(1500 * time.Millisecond)

	r.say("laziergit: lazygit's power, as a native Mac app")
	r.hold(2500 * time.Millisecond)
	r.finish()
}

// recorder makes the frames of a recording of the window, at frameRate,
// writing a frame when the picture changed, and ffmpeg's list of their
// durations (frames/list.txt).
type recorder struct {
	m       *media
	a       *App
	tt      *ui.Tester
	stage   *stage
	sprite  *image.RGBA
	pad     int
	x, y    float32 // the pointer, in DIPs
	caption *image.RGBA
	frames  []recFrame
	last    [32]byte
}

type recFrame struct {
	file string
	d    time.Duration
}

func (m *media) recorder(a *App, tt *ui.Tester) *recorder {
	bg := gradient(videoW, videoH, rgba(30, 36, 62, 255), rgba(78, 52, 120, 255))
	w, h := mediaW*mediaScale, mediaH*mediaScale
	st := newStage(videoW, videoH, bg, (videoW-w)/2, (videoH-h)/2-24, w, h, mediaScale, false)
	sprite, pad := pointerSprite(mediaScale)
	return &recorder{m: m, a: a, tt: tt, stage: st, sprite: sprite, pad: pad, x: 900, y: 640}
}

// say shows a caption under the window, empty for none.
func (r *recorder) say(text string) {
	if text == "" {
		r.caption = nil
		return
	}
	tt := ui.NewTester(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Column(c).Fill().Center().Children(func() {
			ui.Text(c, text).FontSize(captionFontSize).FontWeight(600).TextColor(ui.RGB(255, 255, 255)).
				Padding(11, 20).Radius(14).Background(ui.RGBA(16, 16, 22, 0.86))
		})
	}, 1000, 80)
	tt.SetScale(mediaScale)
	tt.Frame()
	r.caption = cropOpaque(tt.Image())
}

// shot captures the window as it is, shown for d.
func (r *recorder) shot(d time.Duration) {
	r.tt.Frame()
	img := r.stage.compose(r.tt.Image())
	// The pointer, where the window has it.
	px := r.stage.win.Min.X + int(r.x*mediaScale) - r.pad
	py := r.stage.win.Min.Y + int(r.y*mediaScale) - r.pad
	draw.Draw(img, r.sprite.Bounds().Add(image.Pt(px, py)), r.sprite, image.Point{}, draw.Over)
	if c := r.caption; c != nil {
		cb := c.Bounds()
		at := image.Pt((videoW-cb.Dx())/2, r.stage.win.Max.Y-cb.Dy()-56*mediaScale)
		draw.Draw(img, cb.Sub(cb.Min).Add(at), c, cb.Min, draw.Over)
	}
	sum := sha256.Sum256(img.Pix)
	if len(r.frames) > 0 && sum == r.last {
		r.frames[len(r.frames)-1].d += d
		return
	}
	r.last = sum
	name := fmt.Sprintf("frames/%04d.png", len(r.frames))
	r.m.save(name, img)
	r.frames = append(r.frames, recFrame{name, d})
}

// hold records the window for d, its animations going on.
func (r *recorder) hold(d time.Duration) {
	for elapsed := time.Duration(0); elapsed < d; elapsed += frameDuration {
		r.tt.Advance(frameDuration)
		r.shot(frameDuration)
	}
}

// moveTo moves the pointer to (x, y) over d, easing in and out.
func (r *recorder) moveTo(x, y float32, d time.Duration) {
	x0, y0 := r.x, r.y
	n := max(1, int(d/frameDuration))
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		e := float32(0.5 - math.Cos(t*math.Pi)/2)
		r.x, r.y = x0+(x-x0)*e, y0+(y-y0)*e
		r.tt.Move(r.x, r.y)
		r.tt.Advance(frameDuration)
		r.shot(frameDuration)
	}
}

// dragTo presses where the pointer is, moves to (x, y) over d, and lets go.
func (r *recorder) dragTo(x, y float32, d time.Duration) {
	r.tt.Press(r.x, r.y)
	r.shot(frameDuration)
	r.moveTo(x, y, d)
	r.tt.Release(r.x, r.y)
	r.shot(frameDuration)
}

// clickOn moves the pointer to the element showing text s and clicks it.
func (r *recorder) clickOn(s string) {
	x, y := r.m.center(r.tt, s)
	dist := math.Hypot(float64(x-r.x), float64(y-r.y))
	r.moveTo(x, y, time.Duration(300+dist*0.6)*time.Millisecond)
	r.hold(120 * time.Millisecond)
	r.tt.ClickAt(x, y)
	r.hold(250 * time.Millisecond)
}

// key presses a key, and shows the section it goes to.
func (r *recorder) key(mods ui.Modifiers, k ui.Key, _ string) {
	r.tt.Key(mods, k)
	r.hold(350 * time.Millisecond)
}

// finish writes ffmpeg's list of the frames and their durations.
func (r *recorder) finish() {
	var list strings.Builder
	for _, f := range r.frames {
		fmt.Fprintf(&list, "file '%s'\nduration %.4f\n", filepath.Base(f.file), f.d.Seconds())
	}
	// ffmpeg's concat takes the last file's duration from a repeat of it.
	if n := len(r.frames); n > 0 {
		fmt.Fprintf(&list, "file '%s'\n", filepath.Base(r.frames[n-1].file))
	}
	if err := os.WriteFile(filepath.Join(r.m.out, "frames", "list.txt"), []byte(list.String()), 0o644); err != nil {
		r.m.t.Fatal(err)
	}
	r.m.t.Logf("%d frames", len(r.frames))
}
