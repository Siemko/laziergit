package app

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// Drawing for the media of the README and the launch (media_test.go): the
// window as macOS frames it, over a backdrop, with the pointer and a
// caption, as a recording of the screen shows them. The tester draws the
// window's content alone.

// rgba is a color of 8-bit channels.
func rgba(r, g, b, a uint8) color.NRGBA { return color.NRGBA{r, g, b, a} }

// fillPath fills the path that build draws, at the size of dst, in col.
func fillPath(dst draw.Image, col color.Color, build func(z *vector.Rasterizer)) {
	b := dst.Bounds()
	z := vector.NewRasterizer(b.Dx(), b.Dy())
	build(z)
	z.Draw(dst, b, image.NewUniform(col), image.Point{})
}

// roundRect adds a rectangle of rounded corners to z.
func roundRect(z *vector.Rasterizer, x, y, w, h, r float32) {
	const k = 0.5523 // of a circle's quarter by a cubic
	z.MoveTo(x+r, y)
	z.LineTo(x+w-r, y)
	z.CubeTo(x+w-r+r*k, y, x+w, y+r-r*k, x+w, y+r)
	z.LineTo(x+w, y+h-r)
	z.CubeTo(x+w, y+h-r+r*k, x+w-r+r*k, y+h, x+w-r, y+h)
	z.LineTo(x+r, y+h)
	z.CubeTo(x+r-r*k, y+h, x, y+h-r+r*k, x, y+h-r)
	z.LineTo(x, y+r)
	z.CubeTo(x, y+r-r*k, x+r-r*k, y, x+r, y)
	z.ClosePath()
}

// roundRectReversed is roundRect drawn the other way round.
func roundRectReversed(z *vector.Rasterizer, x, y, w, h, r float32) {
	const k = 0.5523
	z.MoveTo(x+r, y)
	z.CubeTo(x+r-r*k, y, x, y+r-r*k, x, y+r)
	z.LineTo(x, y+h-r)
	z.CubeTo(x, y+h-r+r*k, x+r-r*k, y+h, x+r, y+h)
	z.LineTo(x+w-r, y+h)
	z.CubeTo(x+w-r+r*k, y+h, x+w, y+h-r+r*k, x+w, y+h-r)
	z.LineTo(x+w, y+r)
	z.CubeTo(x+w, y+r-r*k, x+w-r+r*k, y, x+w-r, y)
	z.ClosePath()
}

func circle(z *vector.Rasterizer, cx, cy, r float32) {
	const k = 0.5523
	z.MoveTo(cx+r, cy)
	z.CubeTo(cx+r, cy+r*k, cx+r*k, cy+r, cx, cy+r)
	z.CubeTo(cx-r*k, cy+r, cx-r, cy+r*k, cx-r, cy)
	z.CubeTo(cx-r, cy-r*k, cx-r*k, cy-r, cx, cy-r)
	z.CubeTo(cx+r*k, cy-r, cx+r, cy-r*k, cx+r, cy)
	z.ClosePath()
}

// gradient is a backdrop of size w×h, from one color at its top left to
// another at its bottom right.
func gradient(w, h int, from, to color.NRGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	lerp := func(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
	for y := range h {
		for x := range w {
			t := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
			img.SetRGBA(x, y, color.RGBA{lerp(from.R, to.R, t), lerp(from.G, to.G, t), lerp(from.B, to.B, t), 255})
		}
	}
	return img
}

// stage is a canvas the window shows on, as macOS frames it: its corners
// rounded, its buttons and an edge, and a shadow under it, over a backdrop,
// drawn once for the frames of a recording.
type stage struct {
	base *image.RGBA // the backdrop and the shadow
	mask *image.Alpha
	// material is the window's, under its content: the tester leaves the
	// vibrant sidebar clear.
	material color.Color
	chrome   *image.RGBA // the edge and the buttons
	win      image.Rectangle
	scale    float32
}

// newStage is a canvas w×h, over backdrop (nil for none, clear), with the
// window's content winW×winH at (x, y).
func newStage(w, h int, backdrop image.Image, x, y, winW, winH int, scale float32, dark bool) *stage {
	st := &stage{base: image.NewRGBA(image.Rect(0, 0, w, h)), win: image.Rect(x, y, x+winW, y+winH), scale: scale,
		material: rgba(236, 236, 236, 255)}
	if dark {
		st.material = rgba(44, 44, 46, 255)
	}
	if backdrop != nil {
		draw.Draw(st.base, st.base.Bounds(), backdrop, image.Point{}, draw.Src)
	}
	fx, fy, fw, fh := float32(x), float32(y), float32(winW), float32(winH)
	radius := 10 * scale
	shadow(st.base, fx, fy+14*scale, fw, fh, radius, 40*scale, 0.38)
	st.mask = image.NewAlpha(st.base.Bounds())
	fillPath(st.mask, color.Opaque, func(z *vector.Rasterizer) { roundRect(z, fx, fy, fw, fh, radius) })
	st.chrome = image.NewRGBA(st.base.Bounds())
	edge := rgba(0, 0, 0, 60)
	if dark {
		edge = rgba(255, 255, 255, 40)
	}
	fillPath(st.chrome, edge, func(z *vector.Rasterizer) {
		roundRect(z, fx, fy, fw, fh, radius)
		// The hole: the inner edge drawn the other way round, which the
		// winding takes out.
		roundRectReversed(z, fx+scale, fy+scale, fw-2*scale, fh-2*scale, radius-scale)
	})
	// The close, minimize and zoom buttons, where the hidden title bar
	// puts them.
	for i, col := range []color.NRGBA{rgba(255, 95, 87, 255), rgba(254, 188, 46, 255), rgba(40, 200, 64, 255)} {
		cx, cy := fx+(20+float32(i)*20)*scale, fy+26*scale
		fillPath(st.chrome, col, func(z *vector.Rasterizer) { circle(z, cx, cy, 6*scale) })
	}
	return st
}

// compose is the stage showing the window's content win.
func (st *stage) compose(win image.Image) *image.RGBA {
	out := image.NewRGBA(st.base.Bounds())
	copy(out.Pix, st.base.Pix)
	content := image.NewRGBA(image.Rect(0, 0, st.win.Dx(), st.win.Dy()))
	draw.Draw(content, content.Bounds(), image.NewUniform(st.material), image.Point{}, draw.Src)
	draw.Draw(content, content.Bounds(), win, win.Bounds().Min, draw.Over)
	draw.DrawMask(out, st.win, content, image.Point{}, st.mask, st.win.Min, draw.Over)
	draw.Draw(out, out.Bounds(), st.chrome, image.Point{}, draw.Over)
	return out
}

// shadow darkens dst under a rounded rectangle, blurred by spread.
func shadow(dst draw.Image, x, y, w, h, r, spread float32, alpha float64) {
	b := dst.Bounds()
	// Blurred at a quarter of the size, as the blur hides the difference.
	const k = 4
	small := image.NewAlpha(image.Rect(0, 0, b.Dx()/k+1, b.Dy()/k+1))
	fillPath(small, color.Opaque, func(z *vector.Rasterizer) { roundRect(z, x/k, y/k, w/k, h/k, r/k) })
	blurred := boxBlur(small, int(spread/k/2))
	blurred = boxBlur(blurred, int(spread/k/2))
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			a := float64(blurred.AlphaAt(px/k, py/k).A) / 255 * alpha
			if a == 0 {
				continue
			}
			under := color.NRGBAModel.Convert(dst.At(px, py)).(color.NRGBA)
			// Black over what is there.
			oa := float64(under.A) / 255
			na := a + oa*(1-a)
			if na == 0 {
				continue
			}
			mix := func(c uint8) uint8 { return uint8(math.Round(float64(c) * oa * (1 - a) / na)) }
			dst.Set(px, py, color.NRGBA{mix(under.R), mix(under.G), mix(under.B), uint8(math.Round(na * 255))})
		}
	}
}

// boxBlur blurs an alpha image by a box of radius r, across then down.
func boxBlur(src *image.Alpha, r int) *image.Alpha {
	if r < 1 {
		return src
	}
	b := src.Bounds()
	pass := func(in *image.Alpha, horizontal bool) *image.Alpha {
		out := image.NewAlpha(b)
		n, m := b.Dx(), b.Dy()
		if !horizontal {
			n, m = m, n
		}
		at := func(i, j int) int {
			if horizontal {
				return int(in.AlphaAt(i, j).A)
			}
			return int(in.AlphaAt(j, i).A)
		}
		for j := range m {
			sum := 0
			for i := -r; i <= r; i++ {
				sum += at(max(0, min(i, n-1)), j)
			}
			for i := range n {
				v := uint8(sum / (2*r + 1))
				if horizontal {
					out.SetAlpha(i, j, color.Alpha{v})
				} else {
					out.SetAlpha(j, i, color.Alpha{v})
				}
				sum += at(min(i+r+1, n-1), j) - at(max(i-r, 0), j)
			}
		}
		return out
	}
	return pass(pass(src, true), false)
}

// pointerSprite is the arrow of the mouse pointer, macOS's, black and
// outlined in white, its tip at (pad, pad).
func pointerSprite(scale float32) (img *image.RGBA, pad int) {
	pad = int(4 * scale)
	img = image.NewRGBA(image.Rect(0, 0, int(20*scale)+2*pad, int(26*scale)+2*pad))
	arrow := func(z *vector.Rasterizer, x, y, s float32) {
		pts := [][2]float32{{0, 0}, {0, 16}, {4, 12.5}, {6.8, 18.6}, {9.4, 17.4}, {6.7, 11.4}, {11.5, 11.4}}
		for i, p := range pts {
			px, py := x+p[0]*s*scale, y+p[1]*s*scale
			if i == 0 {
				z.MoveTo(px, py)
			} else {
				z.LineTo(px, py)
			}
		}
		z.ClosePath()
	}
	tip := float32(pad)
	fillPath(img, rgba(255, 255, 255, 255), func(z *vector.Rasterizer) { arrow(z, tip-1.3*scale, tip-2.2*scale, 1.22) })
	fillPath(img, rgba(0, 0, 0, 255), func(z *vector.Rasterizer) { arrow(z, tip, tip, 1) })
	return img, pad
}

// cropOpaque is img cut to the part that isn't clear.
func cropOpaque(img *image.RGBA) *image.RGBA {
	b := img.Bounds()
	lo, hi := image.Pt(b.Max.X, b.Max.Y), b.Min
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > 0 {
				lo.X, lo.Y = min(lo.X, x), min(lo.Y, y)
				hi.X, hi.Y = max(hi.X, x+1), max(hi.Y, y+1)
			}
		}
	}
	if hi.X <= lo.X {
		return img
	}
	return img.SubImage(image.Rectangle{lo, hi}).(*image.RGBA)
}
