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

// tint is a row's colored text: the accent's text color on a row chosen,
// whose background is the accent.
func tint(sel bool, tt *ui.Theme, col ui.Color) ui.Color {
	if sel {
		return tt.AccentText.Alpha(0.82)
	}
	return col
}
