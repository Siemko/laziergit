package app

import (
	"fmt"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/jesseduffield/lazygit/pkg/i18n"
)

func abbreviateHome(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func (a *App) repoTr() *i18n.TranslationSet { return a.repo.Tr() }

// plural is n and the noun, in the plural unless n is 1.
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	if strings.HasSuffix(noun, "sh") || strings.HasSuffix(noun, "ch") {
		return fmt.Sprintf("%d %ses", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// listIn builds a list whose chosen row shows in the accent while the list
// has the keys, hot, and in gray otherwise, as Finder's lists do. Its rows
// tint their text (tint) only while hot.
func listIn(c *ui.Context, hot bool, build func()) {
	// The list is in the same place either way, so that it keeps its
	// focus and scroll as it turns hot.
	th := *c.Theme()
	if !hot || !c.WindowActive() {
		pal := paletteFor(&th)
		th.Accent = pal.selected.Over(pal.pane)
		th.AccentText = th.Text
		// The same gray in a window in the background, rather than the
		// theme's darker inactive accent.
		th.InactiveAccent = th.Accent
	}
	ui.Themed(c, &th).Background(ui.Transparent).Grow(1).MinHeight(0).Children(build)
}

// hot reports whether the list of pane p has the keys, in an active window.
func (a *App) hot(c *ui.Context, p pane) bool { return a.pane == p && c.WindowActive() }

// fitPath is path, shortened from its start ("…/widgets/Chart.tsx") to fit
// width at size, keeping the file's name whole.
func fitPath(c *ui.Context, path string, width, size float32, weight int) string {
	fits := func(s string) bool {
		w, _ := c.MeasureText(0, ui.Span{Text: s, Size: size, Weight: weight})
		return w <= width
	}
	if width <= 0 || fits(path) {
		return path
	}
	parts := strings.Split(path, "/")
	for i := 1; i < len(parts); i++ {
		if s := "…/" + strings.Join(parts[i:], "/"); fits(s) || i == len(parts)-1 {
			return s
		}
	}
	return path
}

// tint is a row's colored text: the accent's text color on a row chosen,
// whose background is the accent.
func tint(sel bool, tt *ui.Theme, col ui.Color) ui.Color {
	if sel {
		return tt.AccentText.Alpha(0.92)
	}
	return col
}
