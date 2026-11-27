package app

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// linear opens, in History, a repository of commits one to five, each
// adding a file, five on top and chosen.
func linear(t *testing.T) (*App, *ui.Tester, string) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-test"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Ada Lovelace")
	git(t, dir, "config", "user.email", "ada@example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		write(t, dir, name+".txt", name+"\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", name)
	}
	a, tt, _ := testerFor(t, dir)
	tt.Key(0, ui.Key2)
	tt.Frame()
	if a.section != secHistory {
		t.Fatal("not in History")
	}
	return a, tt, dir
}

func log(t *testing.T, dir string) string {
	return strings.ReplaceAll(git(t, dir, "log", "--format=%s"), "\n", " ")
}

// press sends keys, a frame after each.
func press(tt *ui.Tester, mods ui.Modifiers, keys ...ui.Key) {
	for _, k := range keys {
		tt.Key(mods, k)
		tt.Frame()
	}
}

// confirm checks the confirmation asks title, and clicks its button.
func confirmWith(t *testing.T, a *App, tt *ui.Tester, title, button string) {
	t.Helper()
	if a.dialog == nil || a.dialog.title != title {
		t.Fatalf("confirmation %+v, want %q", a.dialog, title)
	}
	if err := tt.Click(button); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
}

func TestHistoryRangeDrop(t *testing.T) {
	a, tt, dir := linear(t)
	press(tt, ui.Shift, ui.KeyDown) // five, four
	press(tt, 0, ui.KeyD)
	confirmWith(t, a, tt, "Drop 2 commits?", "Drop")
	if got := log(t, dir); got != "three two one" {
		t.Fatalf("log: %s", got)
	}
}

func TestHistoryRangeCopy(t *testing.T) {
	_, tt, _ := linear(t)
	press(tt, ui.Shift, ui.KeyJ, ui.KeyJ) // five, four, three
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("3 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
}

// A move without Shift leaves the range for the one commit moved to.
func TestHistoryPlainMoveEndsRange(t *testing.T) {
	a, tt, _ := linear(t)
	press(tt, ui.Shift, ui.KeyDown) // five, four
	press(tt, 0, ui.KeyDown)        // three
	press(tt, 0, ui.KeyD)
	if a.dialog == nil || a.dialog.title != "Drop “three”?" {
		t.Fatalf("confirmation %+v", a.dialog)
	}
}

// v holds the range while j and k move, as in lazygit; v again lets go.
func TestHistoryStickyRange(t *testing.T) {
	_, tt, _ := linear(t)
	press(tt, 0, ui.KeyV, ui.KeyJ, ui.KeyJ) // five, four, three
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("3 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
	press(tt, 0, ui.KeyV, ui.KeyJ) // two alone
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("4 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
}

func TestHistoryRangeFixup(t *testing.T) {
	_, tt, dir := linear(t)
	press(tt, ui.Shift, ui.KeyDown) // five, four
	press(tt, 0, ui.KeyF)           // into three
	if got := log(t, dir); got != "three two one" {
		t.Fatalf("log: %s", got)
	}
	if files := git(t, dir, "show", "--format=", "--name-only", "HEAD"); files != "five.txt\nfour.txt\nthree.txt" {
		t.Fatalf("three's files: %q", files)
	}
}

// A range moves as a block, and stays chosen where it went.
func TestHistoryRangeMove(t *testing.T) {
	a, tt, dir := linear(t)
	press(tt, 0, ui.KeyJ)           // four
	press(tt, ui.Shift, ui.KeyDown) // four, three
	press(tt, ui.Ctrl, ui.KeyJ)
	if got := log(t, dir); got != "five two four three one" {
		t.Fatalf("log: %s", got)
	}
	press(tt, 0, ui.KeyD)
	if a.dialog == nil || a.dialog.title != "Drop 2 commits?" {
		t.Fatalf("range not kept: %+v", a.dialog)
	}
}

func TestHistoryShiftClick(t *testing.T) {
	_, tt, _ := linear(t)
	r, ok := tt.Find("three") // its row takes the click
	if !ok {
		t.Fatal("no three")
	}
	tt.ClickAtWith(ui.Shift, r.X+r.W/2, r.Y+r.H/2) // five to three
	tt.Frame()
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("3 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
}

// center is the middle of the row showing text s.
func center(t *testing.T, tt *ui.Tester, s string) (float32, float32) {
	t.Helper()
	r, ok := tt.Find(s)
	if !ok {
		t.Fatalf("no %q in %q", s, tt.Texts())
	}
	return r.X + r.W/2, r.Y + r.H/2
}

// chosen is the subjects of the commits chosen in History, newest first.
func chosen(a *App) string {
	var out []string
	for _, i := range a.chosenCommits() {
		out = append(out, a.snap.Commits[i].Name)
	}
	return strings.Join(out, " ")
}

// A Shift-click extends from the commit j and k moved to, which the list
// never chose itself.
func TestHistoryShiftClickAfterKeys(t *testing.T) {
	a, tt, _ := linear(t)
	press(tt, 0, ui.KeyJ) // four
	x, y := center(t, tt, "two")
	tt.ClickAtWith(ui.Shift, x, y)
	tt.Frame()
	if got := chosen(a); got != "four three two" {
		t.Fatalf("chosen %q", got)
	}
	// Shift with the keys goes on from the same anchor.
	press(tt, ui.Shift, ui.KeyUp)
	if got := chosen(a); got != "four three" {
		t.Fatalf("chosen %q after ⇧↑", got)
	}
}

// A click chooses one commit, ending a range and v.
func TestHistoryClickEndsRange(t *testing.T) {
	a, tt, _ := linear(t)
	press(tt, 0, ui.KeyV, ui.KeyJ, ui.KeyJ)
	x, y := center(t, tt, "two")
	tt.ClickAt(x, y)
	tt.Frame()
	if got := chosen(a); got != "two" || a.commitSticky {
		t.Fatalf("chosen %q, sticky %v", got, a.commitSticky)
	}
}

func TestHistoryCmdClick(t *testing.T) {
	a, tt, _ := linear(t)
	x, y := center(t, tt, "three")
	tt.ClickAtWith(ui.Cmd, x, y)
	tt.Frame()
	if got := chosen(a); got != "five three" {
		t.Fatalf("chosen %q", got)
	}
	press(tt, 0, ui.KeyD)
	if a.dialog != nil {
		t.Fatalf("a drop of commits apart: %+v", a.dialog)
	}
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("2 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
}

func TestHistoryDragSelect(t *testing.T) {
	a, tt, dir := linear(t)
	x, y := center(t, tt, "four")
	_, y2 := center(t, tt, "two")
	tt.Press(x, y)
	tt.Frame()
	for _, yy := range []float32{y + 8, (y + y2) / 2, y2} {
		tt.Move(x, yy)
		tt.Frame()
	}
	tt.Release(x, y2)
	tt.Frame()
	if got := chosen(a); got != "four three two" {
		t.Fatalf("chosen %q", got)
	}
	snapshot(t, tt, "history-range")
	tt.SetDark(true)
	snapshot(t, tt, "history-range-dark")
	tt.SetDark(false)
	// The range bar acts on it.
	if !tt.HasText("3 commits") {
		t.Fatalf("no range bar in %q", tt.Texts())
	}
	if err := tt.Click("Drop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	confirmWith(t, a, tt, "Drop 3 commits?", "Drop")
	if got := log(t, dir); got != "five one" {
		t.Fatalf("log: %s", got)
	}
	if got := chosen(a); got == "" {
		t.Fatal("nothing chosen after the drop")
	}
}

func TestHistoryRangeBarMoveAndClear(t *testing.T) {
	a, tt, dir := linear(t)
	press(tt, 0, ui.KeyJ)
	press(tt, ui.Shift, ui.KeyJ) // four, three
	if err := tt.Click("Move down (⌃j)"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := log(t, dir); got != "five two four three one" {
		t.Fatalf("log: %s", got)
	}
	if got := chosen(a); got != "four three" {
		t.Fatalf("chosen %q after the move", got)
	}
	press(tt, 0, ui.KeyEscape) // the commit the keys are on, three
	if got := chosen(a); got != "three" || tt.HasText("2 commits") {
		t.Fatalf("chosen %q after esc", got)
	}
}

func TestHistoryDoubleClickRewords(t *testing.T) {
	a, tt, _ := linear(t)
	x, y := center(t, tt, "three")
	tt.ClickAt(x, y)
	tt.ClickAt(x, y)
	tt.Frame()
	if a.dialog == nil || a.dialog.title != "Reword commit" || a.dialog.value != "three" {
		t.Fatalf("dialog %+v", a.dialog)
	}
}

// Enter goes to the commit's files, as in lazygit.
func TestHistoryEnterShowsFiles(t *testing.T) {
	a, tt, _ := linear(t)
	press(tt, 0, ui.KeyEnter)
	if a.dialog != nil || a.pane != paneFiles {
		t.Fatalf("dialog %+v, pane %v", a.dialog, a.pane)
	}
}
