package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/commands/patch"

	"github.com/Siemko/laziergit/internal/core"
)

// stagingView is a file's diff in the work tree, line by line, whose lines
// are chosen to stage, unstage or discard, as lazygit's staging panel.
// Lines are known by their index in the parsed patch, as core.ApplyLines
// takes them.
type stagingView struct {
	key     string // the file, side and snapshot the diff is of
	path    string
	text    string // the diff
	lines   []*patch.PatchLine
	newNums []int
	oldNums []int
	first   int // the first line shown: the first hunk's header

	cursor int  // the line the keys move
	anchor int  // the other end of the range chosen, -1 for none
	sticky bool // v: moving extends the range

	list ui.ListState

	spans    [][]ui.Span // each line's code, highlighted
	dark     bool        // the theme spans were made for
	numW     float32     // the width of a column of line numbers
	rowWidth float32
}

func (s *stagingView) empty() bool { return len(s.lines) <= s.first }

// set shows a new diff, keeping the cursor where it was when it can, on the
// change at or after it otherwise.
func (s *stagingView) set(path, text string) {
	if text == s.text && path == s.path {
		return
	}
	samePath := path == s.path
	p := patch.Parse(text)
	s.path, s.text, s.lines = path, text, p.Lines()
	s.newNums, s.oldNums = p.LineNumbersOfLines()
	s.first = len(s.lines)
	for i, l := range s.lines {
		if l.Kind == patch.HUNK_HEADER {
			s.first = i
			break
		}
	}
	s.spans = nil
	s.anchor, s.sticky = -1, false
	if !samePath {
		s.cursor = 0
		s.list = ui.ListState{}
	}
	s.cursor = s.changeFrom(max(s.cursor, s.first), 1)
	if s.cursor < 0 {
		s.cursor = s.changeFrom(len(s.lines)-1, -1)
	}
	s.list.ScrollIntoView(max(s.cursor-s.first, 0))
}

// changeFrom is the first changed line from i on, going by step; -1 when
// there is none.
func (s *stagingView) changeFrom(i, step int) int {
	for ; i >= s.first && i < len(s.lines); i += step {
		if s.lines[i].IsChange() {
			return i
		}
	}
	return -1
}

func (s *stagingView) bounds() (lo, hi int) {
	if s.anchor < 0 {
		return s.cursor, s.cursor
	}
	return min(s.anchor, s.cursor), max(s.anchor, s.cursor)
}

// chosen is the changed lines chosen.
func (s *stagingView) chosen() []int {
	if s.cursor < 0 {
		return nil
	}
	lo, hi := s.bounds()
	var out []int
	for i := lo; i <= hi && i < len(s.lines); i++ {
		if s.lines[i].IsChange() {
			out = append(out, i)
		}
	}
	return out
}

// move goes to the next or previous change, extending the range with
// Shift or while v holds it.
func (s *stagingView) move(step int, extend bool) {
	next := s.changeFrom(s.cursor+step, step)
	if next < 0 {
		return
	}
	extend = extend || s.sticky
	if extend && s.anchor < 0 {
		s.anchor = s.cursor
	} else if !extend {
		s.anchor = -1
	}
	s.cursor = next
	s.list.ScrollIntoView(s.cursor - s.first)
}

// hunk is the lines of the hunk line i is in: its header and its last line.
func (s *stagingView) hunk(i int) (header, last int) {
	header = i
	for header > s.first && s.lines[header].Kind != patch.HUNK_HEADER {
		header--
	}
	last = header + 1
	for last+1 < len(s.lines) && s.lines[last+1].Kind != patch.HUNK_HEADER {
		last++
	}
	return header, last
}

// selectHunk chooses the changes of the hunk the cursor is in, as lazygit's
// a does.
func (s *stagingView) selectHunk() {
	header, last := s.hunk(s.cursor)
	from, to := s.changeFrom(header, 1), s.changeFrom(last, -1)
	if from < 0 || to < from {
		return
	}
	s.anchor, s.cursor = from, to
}

func (s *stagingView) toggleRange() {
	s.sticky = !s.sticky
	if s.sticky && s.anchor < 0 {
		s.anchor = s.cursor
	} else if !s.sticky {
		s.anchor = -1
	}
}

// highlight tokenizes the lines with the file's language: the old side's
// lines and the new side's each go on from those before them in their hunk.
func (s *stagingView) highlight(c *ui.Context) {
	tt := c.Theme()
	if s.spans != nil && s.dark == tt.Dark {
		return
	}
	s.dark = tt.Dark
	et := editor.DefaultTheme(tt)
	tok := tokenizerFor(s.path)
	s.spans = make([][]ui.Span, len(s.lines))
	var oldState, newState uint64
	maxCols, maxNum := 0, 1
	for i := s.first; i < len(s.lines); i++ {
		l := s.lines[i]
		if l.Kind == patch.HUNK_HEADER {
			oldState, newState = 0, 0
			continue
		}
		maxNum = max(maxNum, s.newNums[i], s.oldNums[i])
		code := ""
		if len(l.Content) > 0 {
			code = expandTabs(l.Content[1:])
		}
		maxCols = max(maxCols, len(code))
		if tok == nil || l.Kind == patch.NEWLINE_MESSAGE {
			s.spans[i] = []ui.Span{{Text: code, Color: et.Text}}
			continue
		}
		state := &newState
		if l.Kind == patch.DELETION {
			state = &oldState
		}
		var next uint64
		s.spans[i], next = tokenSpans(tok, et, code, *state)
		*state = next
		if l.Kind == patch.CONTEXT {
			oldState = next
		}
	}
	charW, _ := c.MeasureText(0, ui.Span{Text: strings.Repeat("0", 20), Font: codeFont, Size: 12})
	charW /= 20
	s.numW = float32(len(fmt.Sprint(maxNum)))*charW + 14
	s.rowWidth = 2*s.numW + 24 + float32(maxCols)*charW + 24
}

// tokenSpans is a line of code in the colors of its tokens, and the state
// the next line starts in.
func tokenSpans(tok editor.Tokenizer, et *editor.Theme, code string, state uint64) ([]ui.Span, uint64) {
	if tok == nil {
		return []ui.Span{{Text: code, Color: et.Text}}, 0
	}
	tokens, next := tok.Tokenize(code, state, nil)
	var spans []ui.Span
	at := 0
	for _, t := range tokens {
		end := min(t.End, len(code))
		if end <= at {
			continue
		}
		color := et.Tokens[t.Kind].Color
		if color.A == 0 {
			color = et.Text
		}
		spans = append(spans, ui.Span{Text: code[at:end], Color: color, Italic: et.Tokens[t.Kind].Italic})
		at = end
	}
	if at < len(code) {
		spans = append(spans, ui.Span{Text: code[at:], Color: et.Text})
	}
	return spans, next
}

// tokenizerFor is the tokenizer of a file's language, nil for none.
func tokenizerFor(path string) editor.Tokenizer {
	if lang := editor.LanguageFor(path); lang != nil {
		return lang.Tokenizer
	}
	return nil
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// stagingPane shows the staging view of the file's staged or unstaged
// diff, loading it when the file, the side or the repository changes.
func (a *App) stagingPane(c *ui.Context, pal *palette, f *models.File, staged bool) {
	s := &a.stage
	key := fmt.Sprintf("%d:%s:%v", a.gen, f.Path, staged)
	if key != s.key {
		s.key = key
		repo := a.repo
		a.host.Go(func() {
			text, err := repo.FileDiff(f, staged)
			a.host.Post(func() {
				if s.key != key {
					return
				}
				if err != nil {
					a.toastErr(err)
					return
				}
				s.set(f.Path, text)
			})
		})
	}
	if s.path != f.Path {
		return // the diff of another file shows until this one's loads
	}
	if s.empty() {
		emptyState(c, pal, iconChanges, "Nothing to show", "A binary file, or a change of mode.")
		return
	}
	s.highlight(c)
	n := len(s.lines) - s.first
	ui.List(c.Key("patch"), &s.list, n, func(i int) {
		a.patchRow(c, pal, f, staged, s.first+i)
	}).Grow(1).RowWidth(s.rowWidth).FocusBind(&a.pane, paneDiff).FocusRing(false).Label("Diff lines")
	a.selectionBar(c, pal, f, staged)
}

func (a *App) patchRow(c *ui.Context, pal *palette, f *models.File, staged bool, i int) {
	tt := c.Theme()
	s := &a.stage
	l := s.lines[i]
	if l.Kind == patch.HUNK_HEADER {
		a.hunkHeader(c, pal, f, staged, i)
		return
	}
	lo, hi := s.bounds()
	chosen := l.IsChange() && i >= lo && i <= hi
	focused := a.pane == paneDiff
	// The diff's colors are the DiffView's: its hues, mixed as it mixes.
	et := editor.DefaultTheme(tt)
	base := tt.Background
	line, gutter := float32(0.12), float32(0.2)
	if tt.Dark {
		line, gutter = 0.2, 0.3
	}
	var bg, numBg, mark ui.Color
	numColor := pal.muted
	sign := " "
	switch l.Kind {
	case patch.ADDITION:
		bg, numBg, mark, sign = base.MixOklab(et.Added, line), base.MixOklab(et.Added, gutter), et.Added, "+"
	case patch.DELETION:
		bg, numBg, mark, sign = base.MixOklab(et.Deleted, line), base.MixOklab(et.Deleted, gutter), et.Deleted, "−"
	}
	if chosen {
		// Chosen lines keep their color: their gutter fills with it, deep
		// enough for white numbers (4.6:1 and more).
		fill := map[bool][2]ui.Color{false: {ui.Hex("#1a7f37"), ui.Hex("#cf222e")}, true: {ui.Hex("#238636"), ui.Hex("#da3633")}}[tt.Dark]
		mark = fill[0]
		if l.Kind == patch.DELETION {
			mark = fill[1]
		}
		numBg, numColor = mark, ui.RGB(255, 255, 255)
		if !focused {
			// Calmer while the diff hasn't the keys: a deeper tint, its
			// numbers in the color.
			numBg, numColor = base.MixOklab(mark, 0.32), mark
			if tt.Dark {
				numBg, numColor = base.MixOklab(mark, 0.45), ui.RGB(255, 255, 255).Alpha(0.9)
			}
		}
	}
	num := func(n int, show bool) {
		text := ""
		if show {
			text = fmt.Sprint(n)
		}
		ui.Text(c, text).Width(s.numW).Shrink(0).TextAlign(ui.End).Padding(0, 6, 0, 0).Font(codeFont).FontSize(11).TextColor(numColor)
	}
	row := ui.Row(c).Height(20).AlignItems(ui.Stretch).Background(bg).Children(func() {
		ui.Row(c).Shrink(0).AlignItems(ui.Center).Background(numBg).Children(func() {
			// A bar marks the cursor's line.
			bar := ui.Box(c).Width(3).FillHeight()
			if i == s.cursor && focused {
				bar.Background(tt.Accent)
			}
			num(s.oldNums[i], l.Kind != patch.ADDITION)
			num(s.newNums[i], l.Kind != patch.DELETION)
		})
		ui.Text(c, sign).Font(codeFont).FontSize(12).TextColor(mark).Width(20).TextAlign(ui.Center).Shrink(0).AlignSelf(ui.Center)
		ui.RichText(c, s.spans[i]...).Font(codeFont).FontSize(12).SingleLine().AlignSelf(ui.Center).Shrink(0)
	})
	if row.Clicked() {
		a.pane = paneDiff
		if l.IsChange() {
			if row.ClickModifiers()&ui.Shift != 0 {
				if s.anchor < 0 {
					s.anchor = s.cursor
				}
			} else if !s.sticky {
				s.anchor = -1
			}
			s.cursor = i
		}
	}
	row.ContextMenu(func(m *ui.Menu) {
		if l.IsChange() && !chosen {
			s.cursor, s.anchor = i, -1
		}
		verb := "Stage"
		if staged {
			verb = "Unstage"
		}
		if m.Item(verb+" Selected Lines").Shortcut(0, ui.KeySpace).Chosen() {
			a.applyChosen(f, staged, false)
		}
		if m.Item(verb + " Hunk").Chosen() {
			s.cursor = i
			s.selectHunk()
			a.applyChosen(f, staged, false)
		}
		if !staged {
			m.Separator()
			if m.Item("Discard Selected Lines…").Shortcut(0, ui.KeyD).Chosen() {
				a.applyChosen(f, staged, true)
			}
		}
	})
}

func (a *App) hunkHeader(c *ui.Context, pal *palette, f *models.File, staged bool, i int) {
	s := &a.stage
	header, context, _ := strings.Cut(strings.TrimPrefix(s.lines[i].Content, "@@ "), " @@")
	ui.Row(c).Height(30).Padding(0, 10, 0, 12).Gap(10).AlignItems(ui.Center).Background(pal.pill).
		BorderWidth(1, 0, 1, 0).BorderColor(pal.border).Children(func() {
		// The buttons come first, so that they show whatever lines as
		// long as the rows are scroll them sideways.
		hunkAct := func(label string, discard bool) {
			b := ui.ButtonBase(c.Key(label)).Height(22).Padding(0, 9).Radius(5).Center().Label(label).Background(t(c).Background).
				Border(1, pal.border).Shrink(0)
			if b.Hovered() {
				b.Background(pal.hover)
			}
			b.Children(func() { ui.Text(c, label).FontSize(11).FontWeight(500) })
			if b.Clicked() {
				s.cursor = i
				s.selectHunk()
				a.applyChosen(f, staged, discard)
			}
		}
		if !staged {
			hunkAct("Stage Hunk", false)
			hunkAct("Discard", true)
		} else {
			hunkAct("Unstage Hunk", false)
		}
		ui.Text(c, "@@ "+header+" @@").Font(codeFont).FontSize(11).TextColor(pal.muted).Shrink(0)
		ui.Text(c, strings.TrimSpace(context)).Font(codeFont).FontSize(11).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
	})
}

// selectionBar says what is chosen, and acts on it.
func (a *App) selectionBar(c *ui.Context, pal *palette, f *models.File, staged bool) {
	s := &a.stage
	n := len(s.chosen())
	ui.Row(c).Height(50).Shrink(0).Padding(0, 12).Gap(8).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(pal.border).Background(pal.header).Children(func() {
		label := plural(n, "line")
		if s.sticky {
			label += " · range"
		}
		ui.Text(c, label).FontSize(12).TextColor(pal.muted).SingleLine().Shrink(1).MinWidth(0)
		ui.Spacer(c)
		if !staged {
			if ui.Button(c, "Discard").Disabled(n == 0).Tooltip("Discard the lines chosen (d)").Clicked() {
				a.applyChosen(f, staged, true)
			}
		}
		verb := "Stage Lines"
		if staged {
			verb = "Unstage Lines"
		}
		if defaultButton(c, verb, n > 0).Tooltip(verb + " (space)").Clicked() {
			a.applyChosen(f, staged, false)
		}
	})
}

// applyChosen stages, unstages or discards the lines chosen.
func (a *App) applyChosen(f *models.File, staged, discard bool) {
	s := &a.stage
	chosen := s.chosen()
	if len(chosen) == 0 {
		return
	}
	text := s.text
	action, doing := core.StageLines, "Staging lines…"
	switch {
	case discard:
		action, doing = core.DiscardLines, "Discarding lines…"
	case staged:
		action, doing = core.UnstageLines, "Unstaging lines…"
	}
	apply := func() {
		s.anchor, s.sticky = -1, false
		a.run(doing, func(r *core.Repo) error { return r.ApplyLines(f, text, chosen, action) })
	}
	if discard {
		a.confirm("Discard "+plural(len(chosen), "line")+"?", "They are lost from "+f.Path+"; this can't be undone.", "Discard Lines", apply)
		return
	}
	apply()
}
