package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	// lazygit's rebases run this binary as git's sequence editor.
	if HandleDaemon() {
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
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// newRepo makes a repository of commits one, two and three, each adding a
// file, and opens it.
func newRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-test"))
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "commit.gpgsign", "false")
	for _, name := range []string{"one", "two", "three"} {
		write(t, dir, name+".txt", name+"\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", name)
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	r, err := Open(dir, Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, dir
}

func subjects(t *testing.T, dir string) []string {
	return strings.Split(git(t, dir, "log", "--format=%s"), "\n")
}

func load(t *testing.T, r *Repo) *Snapshot {
	t.Helper()
	s, err := r.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestLoadStageCommit(t *testing.T) {
	r, dir := newRepo(t)
	s := load(t, r)
	if len(s.Commits) != 3 || s.Commits[0].Name != "three" {
		t.Fatalf("commits: %v", s.Commits)
	}
	if s.Head == nil || s.Head.Name != "main" {
		t.Fatalf("head: %+v", s.Head)
	}
	if len(s.Graph) != 3 {
		t.Fatalf("graph rows: %d", len(s.Graph))
	}

	write(t, dir, "one.txt", "one\nmore\n")
	write(t, dir, "new.txt", "new\n")
	s = load(t, r)
	if len(s.Files) != 2 {
		t.Fatalf("files: %v", s.Files)
	}
	for _, f := range s.Files {
		if f.Path == "one.txt" {
			if d, _ := r.FileDiff(f, false); !strings.Contains(d, "+more") {
				t.Fatalf("diff of one.txt: %q", d)
			}
		}
	}
	if err := r.ToggleAllStaged(s.Files); err != nil {
		t.Fatal(err)
	}
	if err := r.Commit("four", "the body", false); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"four", "three", "two", "one"}) {
		t.Fatalf("log: %v", got)
	}
	if body := git(t, dir, "log", "-1", "--format=%b"); body != "the body" {
		t.Fatalf("body: %q", body)
	}
}

// The history rewriting below goes through lazygit's interactive rebase and
// its daemon: the test proves the app can host it.
func TestRewriteHistory(t *testing.T) {
	r, dir := newRepo(t)
	s := load(t, r)

	if err := r.DropCommit(s.Commits, 1); err != nil { // "two"
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"three", "one"}) {
		t.Fatalf("after drop: %v", got)
	}

	s = load(t, r)
	if err := r.MoveCommit(s.Commits, 0, 1); err != nil { // "three" down
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"one", "three"}) {
		t.Fatalf("after move: %v", got)
	}

	s = load(t, r)
	if err := r.RewordCommit(s.Commits, 1, "THREE", ""); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"one", "THREE"}) {
		t.Fatalf("after reword: %v", got)
	}

	s = load(t, r)
	if err := r.FixupCommit(s.Commits, 0); err != nil { // "one" into "THREE"
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"THREE"}) {
		t.Fatalf("after fixup: %v", got)
	}
	if files := git(t, dir, "ls-files"); files != "one.txt\nthree.txt" {
		t.Fatalf("files after fixup: %q", files)
	}
}

func TestBranchesAndStash(t *testing.T) {
	r, dir := newRepo(t)
	if err := r.NewBranch("feature", "HEAD~1"); err != nil {
		t.Fatal(err)
	}
	s := load(t, r)
	if s.Head.Name != "feature" || len(s.Commits) != 2 {
		t.Fatalf("head %q, %d commits", s.Head.Name, len(s.Commits))
	}
	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "one.txt", "changed\n")
	if err := r.StashAll("wip"); err != nil {
		t.Fatal(err)
	}
	s = load(t, r)
	if len(s.Files) != 0 || len(s.Stash) != 1 {
		t.Fatalf("files %d, stash %d", len(s.Files), len(s.Stash))
	}
	files, err := r.StashFiles(s.Stash[0])
	if err != nil || len(files) != 1 || files[0].Path != "one.txt" {
		t.Fatalf("stash files %v, %v", files, err)
	}
	if err := r.StashPop(s.Stash[0]); err != nil {
		t.Fatal(err)
	}
	if s = load(t, r); len(s.Files) != 1 {
		t.Fatalf("files after pop: %d", len(s.Files))
	}
}

func TestGraphOfMerge(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side", "HEAD~1")
	write(t, dir, "side.txt", "side\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "side")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	s := load(t, r)
	if !s.Commits[0].IsMerge() {
		t.Fatalf("first commit is not the merge: %v", s.Commits[0].Name)
	}
	// The merge's second parent takes a column of its own.
	cols := map[int16]bool{}
	for _, row := range s.Graph {
		for _, p := range row {
			if p.Visible() {
				cols[p.From], cols[p.To] = true, true
			}
		}
	}
	if len(cols) < 2 {
		t.Fatalf("graph has %d columns", len(cols))
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
