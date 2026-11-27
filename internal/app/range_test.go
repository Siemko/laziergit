package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
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

// shared is a host's copies of each repository, as the app's windows
// share them, and the windows drawn again as they change.
type shared struct {
	copies  map[string]*Copies
	testers []*ui.Tester
	stale   bool
}

func (s *shared) host(h *Host) {
	h.Copies = func(dir string) *Copies {
		if s.copies[dir] == nil {
			s.copies[dir] = &Copies{}
		}
		return s.copies[dir]
	}
	// The app's host draws its windows again; here, the next press does.
	h.CopiesChanged = func() { s.stale = true }
	// The app's host runs it on the UI thread, as the tests run all.
	h.Main = func(fn func()) { fn() }
}

// redraw draws the windows again after a change of the copies, as the
// app's host does.
func (s *shared) redraw() {
	if s.stale {
		s.stale = false
		for _, tt := range s.testers {
			tt.Frame()
		}
	}
}

// window opens dir in a window of the shared host.
func (s *shared) window(t *testing.T, dir string) (*App, *ui.Tester) {
	a, tt, _ := testerFor(t, dir)
	s.host(&a.host)
	tt.Frame()
	s.testers = append(s.testers, tt)
	return a, tt
}

// Commits copied in a repository's window paste in the window of one of
// its worktrees, and no other repository's.
func TestCopiesSharedByWorktrees(t *testing.T) {
	a, tt, dir := linear(t)
	s := &shared{copies: map[string]*Copies{}}
	s.host(&a.host)
	wt := filepath.Join(t.TempDir(), "fix")
	git(t, dir, "worktree", "add", "-q", "-b", "fix", wt, "main~2")
	other := t.TempDir()
	git(t, other, "init", "-q", "-b", "main")
	git(t, other, "commit", "-q", "--allow-empty", "-m", "elsewhere")

	press(tt, ui.Shift, ui.KeyDown) // five, four
	press(tt, ui.Shift, ui.KeyC)
	if !tt.HasText("2 commits copied") {
		t.Fatalf("no copies in %q", tt.Texts())
	}
	b, bt := s.window(t, wt)
	c, ct := s.window(t, other)
	if !bt.HasText("2 commits copied from main in " + filepath.Base(dir)) {
		t.Fatalf("the worktree's window has no copies: %q", bt.Texts())
	}
	if ct.HasText("2 commits copied") || c.hasCopies() {
		t.Fatal("another repository's window has the copies")
	}
	snapshot(t, bt, "copies-in-worktree")
	press(bt, ui.Shift, ui.KeyV)
	confirmWith(t, b, bt, "Cherry-pick 2 commits onto fix?", "Cherry-Pick")
	if got := log(t, wt); got != "five four three two one" {
		t.Fatalf("fix: %s", got)
	}
	// Pasted, they are gone from every window of the repository.
	tt.Frame()
	if a.hasCopies() || tt.HasText("2 commits copied") {
		t.Fatalf("copies left: %q", tt.Texts())
	}
}

// worktreeWindows opens linear's repository, and a worktree of it on fix
// at main, in two windows sharing their copies.
func worktreeWindows(t *testing.T) (a, b *App, at, bt *ui.Tester, dir, wt string, s *shared) {
	a, at, dir = linear(t)
	s = &shared{copies: map[string]*Copies{}}
	s.host(&a.host)
	s.testers = append(s.testers, at)
	wt = filepath.Join(t.TempDir(), "fix")
	git(t, dir, "worktree", "add", "-q", "-b", "fix", wt, "main")
	b, bt = s.window(t, wt)
	press(bt, 0, ui.Key2)
	return a, b, at, bt, dir, wt, s
}

// pressIn sends keys to one window, then draws every window again, as the
// app's host does when the copies changed.
func (s *shared) press(tt *ui.Tester, mods ui.Modifiers, keys ...ui.Key) {
	for _, k := range keys {
		press(tt, mods, k)
		s.redraw()
	}
}

// A paste empties the copies only while they are those it pasted.
func TestPasteKeepsNewerCopies(t *testing.T) {
	a, b, at, bt, _, wt, s := worktreeWindows(t)
	git(t, wt, "reset", "-q", "--hard", "HEAD~2") // fix at three
	b.refresh(false)
	bt.Frame()
	s.press(at, 0, ui.KeyJ)        // four
	s.press(at, ui.Shift, ui.KeyC) // copied
	s.press(bt, ui.Shift, ui.KeyV) // the paste of four asks
	// Meanwhile, a copy of five in the first window.
	s.press(at, 0, ui.KeyK)
	s.press(at, ui.Shift, ui.KeyC)
	confirmWith(t, b, bt, "Cherry-pick 1 commit onto fix?", "Cherry-Pick")
	if got := log(t, wt); got != "four three two one" {
		t.Fatalf("fix: %s", got)
	}
	if got := copied(a); got != "five" {
		t.Fatalf("copies after the paste: %q", got)
	}
}

// copied is the subjects of the commits copied, in their order.
func copied(a *App) string {
	var out []string
	for _, cc := range a.copies().commits {
		out = append(out, cc.commit.Name)
	}
	return strings.Join(out, " ")
}

// A commit copied in one worktree uncopies in another, whose log it is in
// too; a range there copies whole, those copied already with the others.
func TestCopiesAcrossLogs(t *testing.T) {
	a, b, at, bt, _, _, s := worktreeWindows(t)
	s.press(at, ui.Shift, ui.KeyC) // five, from main's window
	if !b.isCopied(b.snap.Commits[0]) {
		t.Fatal("five isn't copied in the worktree's window")
	}
	s.press(bt, ui.Shift, ui.KeyC) // uncopies it
	if a.hasCopies() {
		t.Fatalf("still copied: %q", copied(a))
	}
	s.press(at, ui.Shift, ui.KeyC)    // five again, from main's log
	s.press(bt, ui.Shift, ui.KeyDown) // five and four, in the worktree
	s.press(bt, ui.Shift, ui.KeyC)
	if got := copied(a); got != "five four" {
		t.Fatalf("copied %q", got)
	}
	if !at.HasText("2 commits copied from fix in fix") {
		t.Fatalf("banner: %q", at.Texts())
	}
	// Uncopied, the copies say nothing more of where they came from.
	s.press(bt, ui.Shift, ui.KeyC)
	if cp := a.copies(); cp.source != "" || cp.label != "" || cp.worktree != "" {
		t.Fatalf("copies left %+v", cp)
	}
}

// Copies from a log that changed since take their places in it as it is
// now, which order the paste: a parent before its child.
func TestCopiesFollowTheirLog(t *testing.T) {
	a, tt, dir := linear(t)
	log1 := slices.Clone(a.snap.Commits) // five four three two one
	src := a.copySource("feature")
	cp := a.copies()
	cp.add(src, log1, []int{1}) // four, at 1
	// Two commits on top: four is at 3 now, five at 2.
	for _, name := range []string{"six", "seven"} {
		write(t, dir, name+".txt", name+"\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", name)
	}
	a.refresh(false)
	tt.Frame()
	log2 := a.snap.Commits
	cp.add(src, log2, []int{2})
	if got := places(cp); got != "four@3 five@2" {
		t.Fatalf("copies %s", got)
	}
	// A log that doesn't load them, as a branch's past its 200 commits,
	// keeps them, older than those it has, in their order.
	cp.add(src, log2[:2], []int{0})
	if got := places(cp); got != "five@2 four@3 seven@0" {
		t.Fatalf("copies %s", got)
	}
}

// A reflog repeats commits: a copy keeps the row it was copied at.
func TestCopiesKeepTheirReflogRow(t *testing.T) {
	a, _, _ := linear(t)
	five, four := a.snap.Commits[0], a.snap.Commits[1]
	reflog := []*models.Commit{five, four, five} // five again, older
	src := a.copySource("the reflog")
	for _, rows := range [][]int{{2, 1}, {1, 2}} {
		cp := &Copies{}
		for _, i := range rows {
			cp.add(src, reflog, []int{i})
		}
		if got := places(cp); !strings.Contains(got, "five@2") || !strings.Contains(got, "four@1") {
			t.Fatalf("copied %v: %s", rows, got)
		}
	}
}

// places is the copies, each at its place in its log.
func places(cp *Copies) string {
	var out []string
	for _, cc := range cp.commits {
		out = append(out, fmt.Sprintf("%s@%d", cc.commit.Name, cc.index))
	}
	return strings.Join(out, " ")
}
