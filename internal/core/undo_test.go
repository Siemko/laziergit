package core

import (
	"slices"
	"strings"
	"testing"
)

func undo(t *testing.T, r *Repo, redo bool) UndoStep {
	t.Helper()
	s, err := r.FindUndo(redo)
	if err != nil {
		t.Fatalf("find (redo %v): %v", redo, err)
	}
	if err := r.ApplyUndo(s, redo, HasTrackedChanges(load(t, r).Files)); err != nil {
		t.Fatalf("apply %+v (redo %v): %v", s, redo, err)
	}
	return s
}

func TestUndoRedoCommitAndCheckout(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "four.txt", "four\n")
	git(t, dir, "add", ".")
	if err := r.Commit("four", "", false); err != nil {
		t.Fatal(err)
	}
	// Undoing the commit is a soft reset: its changes stay staged.
	if s := undo(t, r, false); s.Kind != UndoCommit {
		t.Fatalf("undid %+v", s)
	}
	if got := subjects(t, dir); got[0] != "three" {
		t.Fatalf("log after undo: %v", got)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "A  four.txt" {
		t.Fatalf("status after undo: %q", st)
	}
	// Redo puts the commit back.
	undo(t, r, true)
	if got := subjects(t, dir); got[0] != "four" {
		t.Fatalf("log after redo: %v", got)
	}
	if _, err := r.FindUndo(true); err == nil {
		t.Fatal("redo with nothing undone")
	}

	// A checkout undoes to the branch it left.
	if err := r.NewBranch("side", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if s := undo(t, r, false); s.Kind != UndoCheckout || s.From != "main" {
		t.Fatalf("undid %+v", s)
	}
	if head := git(t, dir, "branch", "--show-current"); head != "main" {
		t.Fatalf("on %q", head)
	}
	// Undo again goes on to the step before: the commit, already redone.
	if s := undo(t, r, false); s.Kind != UndoCommit {
		t.Fatalf("second undo %+v", s)
	}
	if got := subjects(t, dir); got[0] != "three" {
		t.Fatalf("log after the second undo: %v", got)
	}
}

func TestUndoRebaseStashesChanges(t *testing.T) {
	r, dir := newRepo(t)
	if err := r.DropCommit(load(t, r).Commits, 1); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "one.txt", "local change\n")
	if s := undo(t, r, false); s.Kind != UndoRebase {
		t.Fatalf("undid %+v", s)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"three", "two", "one"}) {
		t.Fatalf("log after undoing the drop: %v", got)
	}
	if got := strings.TrimSpace(readFile(t, dir, "one.txt")); got != "local change" {
		t.Fatalf("local change lost: %q", got)
	}
	if stash := git(t, dir, "stash", "list"); stash != "" {
		t.Fatalf("stash left: %q", stash)
	}
}

func TestCherryPick(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "feature", "HEAD~2")
	for _, n := range []string{"f1", "f2"} {
		write(t, dir, n+".txt", n+"\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", n)
	}
	git(t, dir, "checkout", "-q", "main")
	s := load(t, r)
	var feature = s.Branches[0]
	for _, b := range s.Branches {
		if b.Name == "feature" {
			feature = b
		}
	}
	commits, graph, err := r.BranchCommits(feature, 50)
	if err != nil || len(commits) != 3 || len(graph) != 3 || commits[0].Name != "f2" {
		t.Fatalf("feature's commits %v %v", commits, err)
	}
	// Both, given newest first, land oldest first.
	if err := r.CherryPick(commits[:2]); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got[:3], []string{"f2", "f1", "three"}) {
		t.Fatalf("log: %v", got)
	}
}

func TestCherryPickConflict(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "feature", "HEAD~1")
	write(t, dir, "two.txt", "feature's two\n")
	git(t, dir, "commit", "-q", "-am", "feature changes two")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "two.txt", "main's two\n")
	git(t, dir, "commit", "-q", "-am", "main changes two")

	feature := load(t, r).Branches[1]
	commits, _, _ := r.BranchCommits(feature, 1)
	if err := r.CherryPick(commits); !IsConflict(err) {
		t.Fatalf("cherry-pick: %v", err)
	}
	s := load(t, r)
	if !s.State.CherryPicking {
		t.Fatalf("state %+v", s.State)
	}
	f := fileNamed(t, s, "two.txt")
	content, _ := r.ReadFile(f.Path)
	if err := r.WriteFile(f.Path, ParseConflicts(content, 0).Resolve(0, PickTheirs)); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkResolved(f); err != nil {
		t.Fatal(err)
	}
	if err := r.ContinueOperation(load(t, r).State); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir)[0]; got != "feature changes two" {
		t.Fatalf("HEAD is %q", got)
	}
}

func TestUndoMergeAndCherryPick(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "feature", "HEAD~1")
	write(t, dir, "f.txt", "f\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "f")
	git(t, dir, "checkout", "-q", "main")
	if err := r.Merge("feature"); err != nil {
		t.Fatal(err)
	}
	if s := undo(t, r, false); s.Kind != UndoMerge {
		t.Fatalf("undid %+v", s)
	}
	if got := subjects(t, dir)[0]; got != "three" {
		t.Fatalf("after undoing the merge HEAD is %q", got)
	}
	commits, _, _ := r.BranchCommits(load(t, r).Branches[1], 1)
	if err := r.CherryPick(commits); err != nil {
		t.Fatal(err)
	}
	if s := undo(t, r, false); s.Kind != UndoCommit || subjects(t, dir)[0] != "three" {
		t.Fatalf("undid %+v, HEAD %q", s, subjects(t, dir)[0])
	}
}
