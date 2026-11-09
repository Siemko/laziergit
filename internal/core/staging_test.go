package core

import (
	"strings"
	"testing"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/commands/patch"
)

// linesMatching returns the indices of the diff's changed lines that
// contain s.
func linesMatching(diff string, s ...string) []int {
	var out []int
	for i, l := range patch.Parse(diff).Lines() {
		for _, want := range s {
			if l.IsChange() && strings.Contains(l.Content, want) {
				out = append(out, i)
			}
		}
	}
	return out
}

func fileNamed(t *testing.T, s *Snapshot, path string) *models.File {
	t.Helper()
	for _, f := range s.Files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("no %s in %v", path, s.Files)
	return nil
}

const original = "alpha\nbeta\ngamma\ndelta\nepsilon\nzeta\neta\ntheta\niota\nkappa\n"

func TestStageUnstageDiscardLines(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "greek.txt", original)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "greek")
	// Two changes far apart, so two hunks, and a replaced line.
	write(t, dir, "greek.txt", strings.NewReplacer("beta\n", "BETA\n", "iota\n", "iota\nlambda\n").Replace(original))

	f := fileNamed(t, load(t, r), "greek.txt")
	diff := r.FileDiff(f, false)
	// Stage the change of beta alone: its deletion and its addition.
	if err := r.ApplyLines(f, diff, linesMatching(diff, "beta", "BETA"), StageLines); err != nil {
		t.Fatal(err)
	}
	staged := git(t, dir, "diff", "--cached")
	if !strings.Contains(staged, "+BETA") || strings.Contains(staged, "lambda") {
		t.Fatalf("staged:\n%s", staged)
	}
	if unstaged := git(t, dir, "diff"); !strings.Contains(unstaged, "+lambda") || strings.Contains(unstaged, "BETA") {
		t.Fatalf("unstaged:\n%s", unstaged)
	}

	// Take the addition of BETA back out, leaving beta's deletion staged.
	f = fileNamed(t, load(t, r), "greek.txt")
	diff = r.FileDiff(f, true)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "BETA"), UnstageLines); err != nil {
		t.Fatal(err)
	}
	staged = git(t, dir, "diff", "--cached")
	if !strings.Contains(staged, "-beta") || strings.Contains(staged, "BETA") {
		t.Fatalf("staged after unstaging a line:\n%s", staged)
	}

	// Throw lambda away from the work tree.
	f = fileNamed(t, load(t, r), "greek.txt")
	diff = r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "lambda"), DiscardLines); err != nil {
		t.Fatal(err)
	}
	if unstaged := git(t, dir, "diff"); strings.Contains(unstaged, "lambda") || !strings.Contains(unstaged, "+BETA") {
		t.Fatalf("unstaged after discarding:\n%s", unstaged)
	}

	// Nothing chosen is an error, not a no-op.
	if err := r.ApplyLines(f, diff, nil, StageLines); err == nil {
		t.Fatal("no error for no lines")
	}
}

func TestStageLinesOfUntrackedFile(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "new.txt", "keep\nleave\n")
	f := fileNamed(t, load(t, r), "new.txt")
	diff := r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "keep"), StageLines); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "show", ":new.txt"); got != "keep" {
		t.Fatalf("index has %q", got)
	}
	// Every line staged is the file staged, added.
	f = fileNamed(t, load(t, r), "new.txt")
	diff = r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "leave"), StageLines); err != nil {
		t.Fatal(err)
	}
	if st := git(t, dir, "status", "--porcelain", "new.txt"); st != "A  new.txt" {
		t.Fatalf("status %q", st)
	}
}
