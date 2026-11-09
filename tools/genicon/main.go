// genicon draws the app's icon, resources/icon.png, with MyGo's renderer:
// a commit graph as the app draws it, on the rounded square of macOS
// icons.
//
//	go run ./tools/genicon
package main

import (
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

const size = 1024

func main() {
	tt := ui.NewTester(view, size, size)
	tt.Frame()
	out := filepath.Join("resources", "icon.png")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		log.Fatal(err)
	}
	log.Println("wrote", out)
}

func view(c *ui.Context) {
	c.Root().Background(ui.Transparent)
	ui.Box(c).Size(size, size).Draw(draw)
}

func draw(p *ui.Painter, r ui.Rect) {
	// The body of a macOS icon: 824 points of the 1024, rounded.
	body := ui.Rect{X: r.X + 100, Y: r.Y + 100, W: 824, H: 824}
	const radius = 186
	p.Shadow(body, radius, 0, 14, 30, 0, ui.RGBA(0, 0, 0, 0.32))
	p.FillGradient(body, ui.LinearGradient{
		Angle: 165,
		Stops: []ui.GradientStop{
			{Color: ui.Hex("#272463"), At: 0},
			{Color: ui.Hex("#1a1744"), At: 0.55},
			{Color: ui.Hex("#2a0f45"), At: 1},
		},
		Oklab: true,
	}, radius)
	p.FillRadialGradient(body, ui.RadialGradient{
		Center: ui.Point{X: 0.25, Y: 0.15},
		Radius: 620,
		Stops: []ui.GradientStop{
			{Color: ui.RGBA(255, 255, 255, 0.16), At: 0},
			{Color: ui.RGBA(255, 255, 255, 0), At: 1},
		},
	}, radius)
	// A hairline at the edge, as glass catches the light.
	p.Stroke(body, ui.RGBA(255, 255, 255, 0.10), radius, 3)

	blue, pink, green := ui.Hex("#60a5fa"), ui.Hex("#f472b6"), ui.Hex("#34d399")
	ring := ui.Hex("#1d1a4d")
	// Centered: the main line left of the middle, the branch right of it.
	x0, x1 := r.X+404, r.X+620
	top, bottom := r.Y+290, r.Y+760
	line := func(c ui.Color, path *ui.Path) { p.StrokePath(path, 46, c) }

	// The main line, and a branch that forks from its first commit and
	// merges back at the top, both ends under their commits.
	line(blue, (&ui.Path{}).MoveTo(x0, top).LineTo(x0, bottom))
	line(pink, (&ui.Path{}).MoveTo(x0, bottom).
		CubeTo(x0, bottom-110, x1, bottom-90, x1, bottom-200).
		LineTo(x1, top+200).
		CubeTo(x1, top+90, x0, top+110, x0, top))

	dot := func(x, y float32, c ui.Color, merge bool) {
		p.FillPath((&ui.Path{}).Circle(x, y, 66), ring)
		if merge {
			p.StrokePath((&ui.Path{}).Circle(x, y, 40), 26, c)
			return
		}
		p.FillPath((&ui.Path{}).Circle(x, y, 50), c)
	}
	dot(x0, top, green, true) // the merge, at the top: HEAD
	dot(x1, (top+bottom)/2, pink, false)
	dot(x0, (top+bottom)/2, blue, false)
	dot(x0, bottom, blue, false)
}
