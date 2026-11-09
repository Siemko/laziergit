package app

import (
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Siemko/laziergit/internal/core"
)

func TestMain(m *testing.M) {
	if core.HandleDaemon() {
		return
	}
	os.Exit(m.Run())
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture makes a repository with some history, a merged side branch, a
// stash, and changes in the work tree.
func fixture(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-test"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Ada Lovelace")
	git(t, dir, "config", "user.email", "ada@example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	commit := func(file, content, msg string) {
		write(t, dir, file, content)
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", msg)
	}
	commit("README.md", "# Demo\n", "Initial commit")
	commit("src/main.go", "package main\n\nfunc main() {}\n", "Add main")
	git(t, dir, "checkout", "-q", "-b", "feature/graph")
	commit("src/graph.go", "package main\n\n// Graph draws.\nfunc graph() {}\n", "Draw the commit graph")
	commit("src/graph.go", "package main\n\n// Graph draws the pipes.\nfunc graph() { pipes() }\n", "Draw pipes as curves")
	git(t, dir, "checkout", "-q", "main")
	commit("docs/guide.md", "Guide\n", "Write the guide")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "Merge branch 'feature/graph'", "feature/graph")
	commit("src/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n", "Say hi")
	git(t, dir, "tag", "v0.1.0")
	git(t, dir, "branch", "release")
	write(t, dir, "notes.txt", "stashed\n")
	git(t, dir, "add", "notes.txt")
	git(t, dir, "stash", "push", "-q", "-m", "half-done notes")
	write(t, dir, "src/main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello, world\")\n}\n")
	write(t, dir, "src/new.go", "package main\n\nfunc added() int { return 42 }\n")
	write(t, dir, "README.md", "# Demo\n\nA demo repository.\n")
	git(t, dir, "add", "README.md")
	return dir
}

// newTester opens the fixture in the app, its work done in the frame that
// asks for it.
func newTester(t *testing.T) (*App, *ui.Tester, string) {
	return testerFor(t, fixture(t))
}

func testerFor(t *testing.T, dir string) (*App, *ui.Tester, string) {
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	a := New(Host{Post: func(fn func()) { fn() }, Go: func(fn func()) { fn() }},
		core.Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	a.Open(dir, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	if a.Repo() == nil {
		t.Fatal("not opened")
	}
	tt := ui.NewTester(a.View, 1280, 800)
	tt.Frame()
	return a, tt, dir
}

// snapshot saves the window as a PNG in $LAZIERGIT_SNAPSHOTS.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	out := os.Getenv("LAZIERGIT_SNAPSHOTS")
	if out == "" {
		return
	}
	tt.Frame()
	f, err := os.Create(filepath.Join(out, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func TestChangesStageAndCommit(t *testing.T) {
	a, tt, dir := newTester(t)
	for _, s := range []string{"Changes", "main.go", "new.go", "README.md", "Fetch", "Push"} {
		if !tt.HasText(s) {
			t.Fatalf("no %q in %q", s, tt.Texts())
		}
	}
	snapshot(t, tt, "changes")

	tt.SetDark(true)
	snapshot(t, tt, "changes-dark")
	tt.SetDark(false)

	// a stages everything, as in lazygit.
	tt.Key(0, ui.KeyA)
	tt.Frame()
	if st := git(t, dir, "status", "--porcelain"); strings.Contains(st, "??") || strings.Contains(st, " M") {
		t.Fatalf("not all staged:\n%s", st)
	}
	// c writes the message; ⌘↩ commits.
	tt.Key(0, ui.KeyC)
	tt.Frame()
	tt.Type("Greet the world")
	tt.Key(ui.Cmd, ui.KeyEnter)
	tt.Frame()
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "Greet the world" {
		t.Fatalf("last commit %q", got)
	}
	if len(a.snap.Files) != 0 || !tt.HasText("Working tree clean") {
		t.Fatalf("files left: %d", len(a.snap.Files))
	}
}

func TestHistoryDropThroughDaemon(t *testing.T) {
	a, tt, dir := newTester(t)
	tt.Key(0, ui.Key2)
	tt.Frame()
	if a.section != secHistory || !tt.HasText("Say hi") || !tt.HasText("Draw pipes as curves") {
		t.Fatalf("history not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "history")
	tt.SetDark(true)
	snapshot(t, tt, "history-dark")
	tt.SetDark(false)

	// j moves to "Merge branch…", k back to "Say hi"; d asks to drop it.
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyK)
	tt.Frame()
	if a.commitSel != 0 {
		t.Fatalf("selection %d", a.commitSel)
	}
	tt.Key(0, ui.KeyD)
	tt.Frame()
	if a.dialog == nil {
		t.Fatal("no confirmation")
	}
	snapshot(t, tt, "drop-confirm")
	if err := tt.Click("Drop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "Merge branch 'feature/graph'" {
		t.Fatalf("after drop, HEAD is %q", got)
	}
}

func TestBranchesStashPalette(t *testing.T) {
	a, tt, dir := newTester(t)
	tt.Key(0, ui.Key3)
	tt.Frame()
	for _, s := range []string{"feature/graph", "release", "main"} {
		if !tt.HasText(s) {
			t.Fatalf("no branch %q", s)
		}
	}
	snapshot(t, tt, "branches")

	tt.Key(0, ui.Key4)
	tt.Frame()
	if !tt.HasText("half-done notes") {
		t.Fatalf("no stash in %q", tt.Texts())
	}
	snapshot(t, tt, "stash")

	tt.Key(ui.Cmd, ui.KeyK)
	tt.Frame()
	tt.Type("pop")
	tt.Frame()
	if !tt.HasText("Pop Stash") {
		t.Fatalf("palette: %q", tt.Texts())
	}
	snapshot(t, tt, "palette")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.palette {
		t.Fatal("palette still open")
	}
	if list := git(t, dir, "stash", "list"); list != "" {
		t.Fatalf("stash not popped: %q", list)
	}
}

func TestStageLines(t *testing.T) {
	a, tt, dir := newTester(t)
	// j goes to src/main.go; Enter takes the keys into its diff.
	tt.Key(0, ui.KeyJ)
	tt.Frame()
	if f := a.selectedFile(); f == nil || f.Path != "src/main.go" {
		t.Fatalf("file %v", a.selectedFile())
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if a.pane != paneDiff {
		t.Fatalf("pane %v", a.pane)
	}
	if !tt.HasText("Stage Hunk") || !tt.HasText("Stage Lines") {
		t.Fatalf("no staging view in %q", tt.Texts())
	}
	snapshot(t, tt, "staging")

	// The cursor starts on the first change, the deletion; ↓ goes to the
	// first addition and ⇧↓ takes the next one too.
	tt.Key(0, ui.KeyDown)
	tt.Key(ui.Shift, ui.KeyDown)
	tt.Frame()
	if n := len(a.stage.chosen()); n != 2 {
		t.Fatalf("%d lines chosen", n)
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	staged := git(t, dir, "diff", "--cached", "src/main.go")
	if !strings.Contains(staged, "+func main() {") || !strings.Contains(staged, `+	fmt.Println("hello, world")`) ||
		strings.Contains(staged, `-func main() { fmt.Println("hi") }`) {
		t.Fatalf("staged:\n%s", staged)
	}
	snapshot(t, tt, "staging-partial")

	// Tab shows the staged side, where space takes the lines back out.
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if !a.showingStaged(a.selectedFile()) || !tt.HasText("Unstage Hunk") {
		t.Fatalf("staged side not shown: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyA)
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	if staged := git(t, dir, "diff", "--cached", "src/main.go"); staged != "" {
		t.Fatalf("still staged:\n%s", staged)
	}

	// d discards the line chosen, after asking: k goes up to the first
	// change, the deletion.
	a.diffStaged = 0
	tt.Frame()
	tt.Key(0, ui.KeyK)
	tt.Key(0, ui.KeyK)
	tt.Key(0, ui.KeyD)
	tt.Frame()
	if err := tt.Click("Discard Lines"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	unstaged := git(t, dir, "diff", "src/main.go")
	if strings.Contains(unstaged, `-func main() { fmt.Println("hi") }`) || !strings.Contains(unstaged, "+func main() {") {
		t.Fatalf("deletion not discarded alone:\n%s", unstaged)
	}
}

// conflictFixture makes a repository whose main and feature branches
// change the same two places of greeting.go.
func conflictFixture(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-test"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Ada Lovelace")
	git(t, dir, "config", "user.email", "ada@example.com")
	commit := func(content, msg string) {
		write(t, dir, "greeting.go", content)
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", msg)
	}
	body := func(hello, bye string) string {
		return "package main\n\nimport \"fmt\"\n\nfunc hello() {\n\tfmt.Println(\"" + hello +
			"\")\n}\n\n// Some lines in between.\n// One.\n// Two.\n// Three.\n// Four.\n// Five.\n// Six.\n// Seven.\n\nfunc bye() {\n\tfmt.Println(\"" + bye + "\")\n}\n"
	}
	commit(body("hello", "bye"), "Greet")
	git(t, dir, "checkout", "-q", "-b", "feature")
	commit(body("hello, feature", "bye, feature"), "Greet from the feature")
	git(t, dir, "checkout", "-q", "main")
	commit(body("hello, main", "bye, main"), "Greet from main")
	return dir
}

func TestResolveConflicts(t *testing.T) {
	a, tt, dir := testerFor(t, conflictFixture(t))
	// Merge feature from the branches: it stops on conflicts, and the app
	// shows them.
	tt.Key(0, ui.Key3)
	tt.Frame()
	for i, b := range a.snap.Branches {
		if b.Name == "feature" {
			a.branchSel = i
		}
	}
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyM)
	tt.Frame()
	if err := tt.Click("Merge"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	f := a.selectedFile()
	if a.section != secChanges || f == nil || f.Path != "greeting.go" || !f.HasInlineMergeConflicts {
		t.Fatalf("section %v, file %+v", a.section, f)
	}
	tt.Frame()
	if !tt.HasText("Conflict 1 of 2") || !tt.HasText("Merging") {
		t.Fatalf("no conflicts in %q", tt.Texts())
	}
	snapshot(t, tt, "conflicts")

	// Enter, then space keeps the current side of the first conflict.
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	content := readFile(t, dir, "greeting.go")
	if !strings.Contains(content, `fmt.Println("hello, main")`) || strings.Contains(content, `"hello, feature"`) ||
		strings.Count(content, "<<<<<<<") != 1 {
		t.Fatalf("after the first:\n%s", content)
	}
	// z undoes it.
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if n := strings.Count(readFile(t, dir, "greeting.go"), "<<<<<<<"); n != 2 {
		t.Fatalf("%d conflicts after undo", n)
	}
	// j goes to the incoming side; b keeps both.
	tt.Key(0, ui.KeyJ)
	tt.Key(0, ui.KeyB)
	tt.Frame()
	content = readFile(t, dir, "greeting.go")
	if !strings.Contains(content, "\"hello, main\")\n\tfmt.Println(\"hello, feature\")") {
		t.Fatalf("both:\n%s", content)
	}
	snapshot(t, tt, "conflicts-one-left")

	// The last one, incoming: the file is staged, and the app offers to
	// continue the merge.
	if err := tt.Click("All Incoming"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if st := git(t, dir, "status", "--porcelain"); st != "M  greeting.go" {
		t.Fatalf("status %q", st)
	}
	if a.dialog == nil || !tt.HasText("All conflicts are resolved") {
		t.Fatalf("no offer to continue: %q", tt.Texts())
	}
	snapshot(t, tt, "conflicts-continue")
	if err := tt.Click("Continue Merge"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if parents := git(t, dir, "log", "-1", "--format=%p"); len(strings.Fields(parents)) != 2 {
		t.Fatalf("HEAD is no merge: parents %q", parents)
	}
	if !a.snap.State.None() {
		t.Fatalf("still %+v", a.snap.State)
	}
}

func readFile(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestCherryPickAndUndo(t *testing.T) {
	a, tt, dir := newTester(t)
	git(t, dir, "stash", "-q", "-u") // a clean tree, as cherry-picks want
	a.refresh(false)
	tt.Key(0, ui.Key3)
	tt.Frame()
	for i, b := range a.snap.Branches {
		if b.Name == "feature/graph" {
			a.branchSel = i
		}
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("Commits on feature/graph") || !tt.HasText("Draw pipes as curves") {
		t.Fatalf("no branch commits in %q", tt.Texts())
	}
	// Enter goes into the branch's commits; C copies the newest two.
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyC)
	tt.Key(0, ui.KeyJ)
	tt.Key(ui.Shift, ui.KeyC)
	tt.Frame()
	if len(a.copied) != 2 || !tt.HasText("2 commits copied from feature/graph") {
		t.Fatalf("copied %d: %q", len(a.copied), tt.Texts())
	}
	snapshot(t, tt, "cherry-pick")

	// Make main a branch that doesn't have them yet.
	git(t, dir, "reset", "-q", "--hard", "HEAD~3")
	a.refresh(false)
	tt.Frame()
	tt.Key(ui.Shift, ui.KeyV)
	tt.Frame()
	if err := tt.Click("Cherry-Pick"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	got := strings.Split(git(t, dir, "log", "-3", "--format=%s"), "\n")
	if got[0] != "Draw pipes as curves" || got[1] != "Draw the commit graph" || len(a.copied) != 0 {
		t.Fatalf("log after paste: %v, copied %d", got, len(a.copied))
	}

	// z undoes the last cherry-pick commit, asking first; Z redoes it.
	tt.Key(0, ui.KeyZ)
	tt.Frame()
	if a.dialog == nil || !tt.HasText("Undo?") {
		t.Fatalf("no undo prompt: %q", tt.Texts())
	}
	snapshot(t, tt, "undo")
	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if head := git(t, dir, "log", "-1", "--format=%s"); head != "Draw the commit graph" {
		t.Fatalf("after undo HEAD is %q", head)
	}
	tt.Key(ui.Shift, ui.KeyZ)
	tt.Frame()
	if err := tt.Click("Redo"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if head := git(t, dir, "log", "-1", "--format=%s"); head != "Draw pipes as curves" {
		t.Fatalf("after redo HEAD is %q", head)
	}
}

func TestWelcomeRecent(t *testing.T) {
	var opened string
	asked := false
	a := New(Host{
		Post:           func(fn func()) { fn() },
		Go:             func(fn func()) { fn() },
		Recent:         func() []string { return []string{"/Users/ada/code/engine", "/Users/ada/code/notes"} },
		OpenRecent:     func(p string) { opened = p },
		OpenRepository: func() { asked = true },
	}, core.Options{})
	tt := ui.NewTester(a.View, 1100, 720)
	if !tt.HasText("engine") || !tt.HasText("notes") || !tt.HasText("Recent") {
		t.Fatalf("no recent repositories: %q", tt.Texts())
	}
	snapshot(t, tt, "welcome")
	if err := tt.Click("notes"); err != nil {
		t.Fatal(err)
	}
	if opened != "/Users/ada/code/notes" {
		t.Fatalf("opened %q", opened)
	}
	if err := tt.Click("Open Repository…"); err != nil || !asked {
		t.Fatalf("open repository: %v, asked %v", err, asked)
	}
}

// asyncHost runs work on goroutines, as the app does, and keeps what it
// posts for the UI thread until pump runs it.
type asyncHost struct{ posts chan func() }

func newAsync(t *testing.T, dir string) (*App, *asyncHost) {
	h := &asyncHost{posts: make(chan func(), 64)}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	a := New(Host{Post: func(fn func()) { h.posts <- fn }, Go: func(fn func()) { go fn() }},
		core.Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	a.Open(dir, nil)
	h.pump(t, func() bool { return a.snap != nil })
	return a, h
}

// pump runs what the app posted until done holds.
func (h *asyncHost) pump(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.After(20 * time.Second)
	for !done() {
		select {
		case fn := <-h.posts:
			fn()
		case <-deadline:
			t.Fatal("timed out")
		}
	}
}

func TestActionsRunInOrder(t *testing.T) {
	a, h := newAsync(t, fixture(t))
	dir := a.Repo().Path
	// Stage everything, then commit at once: the commit must see the stage.
	files := a.snap.Files
	a.run("Staging…", func(r *core.Repo) error { return r.ToggleAllStaged(files) })
	a.run("Committing…", func(r *core.Repo) error { return r.Commit("everything", "", false) })
	h.pump(t, func() bool { return a.busy == 0 && !a.loading })
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Fatalf("left after the commit:\n%s", st)
	}
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "everything" {
		t.Fatalf("HEAD %q", got)
	}
}

func TestCloseWithWorkQueued(t *testing.T) {
	a, h := newAsync(t, fixture(t))
	dir := a.Repo().Path
	files := a.snap.Files
	a.run("Staging…", func(r *core.Repo) error { return r.ToggleAllStaged(files) })
	a.run("Committing…", func(r *core.Repo) error { return r.Commit("never", "", false) })
	a.Close() // must neither block nor panic
	h.pump(t, func() bool { return a.busy == 0 })
	if got := git(t, dir, "log", "-1", "--format=%s"); got == "never" {
		t.Fatal("committed after the window closed")
	}
	a.run("Fetching…", (*core.Repo).Fetch) // refused once closed
	if a.busy != 0 {
		t.Fatal("ran work after closing")
	}
}

func TestCredentialAnswers(t *testing.T) {
	a := New(Host{Post: func(fn func()) { fn() }, Go: func(fn func()) { fn() }}, core.Options{})
	tt := ui.NewTester(a.View, 900, 600)
	_ = tt
	answer := a.opts.AskCredential(0) // a password
	a.dialog.value = "secret"
	a.dialog.onOK(a.dialog.value)
	if got := <-answer; got != "secret\n" {
		t.Fatalf("answer %q", got)
	}
	cancelled := a.opts.AskCredential(0)
	a.dialog.onCancel()
	if got := <-cancelled; got != "\n" {
		t.Fatalf("cancel %q", got)
	}
	// A prompt left open is answered as the window closes, so that git
	// doesn't wait on it with the repository taken.
	pending := a.opts.AskCredential(0)
	a.confirm("Something else?", "", "OK", func() {})
	if a.dialog.kind != dialogCredential {
		t.Fatal("a confirmation replaced the credential prompt")
	}
	a.Close()
	if got := <-pending; got != "\n" {
		t.Fatalf("on close %q", got)
	}
}
