package core

import (
	"slices"
	"testing"
)

// rangeRepo is newRepo with two commits more: five, four, three, two and
// one, newest first.
func rangeRepo(t *testing.T) (*Repo, string) {
	r, dir := newRepo(t)
	for _, name := range []string{"four", "five"} {
		write(t, dir, name+".txt", name+"\n")
		git(t, dir, "add", ".")
		git(t, dir, "commit", "-q", "-m", name)
	}
	return r, dir
}

// A range fixed up goes into the commit below it, whose message stays.
func TestFixupCommits(t *testing.T) {
	r, dir := rangeRepo(t)
	if err := r.FixupCommits(load(t, r).Commits, 1, 2); err != nil { // four, three into two
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"five", "two", "one"}) {
		t.Fatalf("log: %v", got)
	}
	if files := git(t, dir, "show", "--format=", "--name-only", "HEAD~1"); files != "four.txt\nthree.txt\ntwo.txt" {
		t.Fatalf("two's files: %q", files)
	}
	if err := r.FixupCommits(load(t, r).Commits, 1, 2); err == nil { // two, one: none below
		t.Fatal("fixed up into nothing")
	}
}

// A range moves as a block, one place at a time, and not past the log.
func TestMoveCommits(t *testing.T) {
	r, dir := rangeRepo(t)
	if err := r.MoveCommits(load(t, r).Commits, 1, 2, 1); err != nil { // four, three below two
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"five", "two", "four", "three", "one"}) {
		t.Fatalf("after down: %v", got)
	}
	if err := r.MoveCommits(load(t, r).Commits, 2, 3, -1); err != nil { // back above two
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"five", "four", "three", "two", "one"}) {
		t.Fatalf("after up: %v", got)
	}
	if err := r.MoveCommits(load(t, r).Commits, 0, 1, -1); err == nil {
		t.Fatal("moved past the top")
	}
}

func TestDropCommits(t *testing.T) {
	r, dir := rangeRepo(t)
	if err := r.DropCommits(load(t, r).Commits, 1, 2); err != nil { // four, three
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"five", "two", "one"}) {
		t.Fatalf("log: %v", got)
	}
}
