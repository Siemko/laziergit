package core

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Two repositories open in one process take turns: each command runs with
// its own repository current, whichever opened last.
func TestTwoRepositories(t *testing.T) {
	a, dirA := newRepo(t)
	b, dirB := newRepo(t)
	// The same untracked file in both; b opened last, so the process is
	// in b's work tree when a discards its copy.
	write(t, dirA, "scratch.txt", "a\n")
	write(t, dirB, "scratch.txt", "b\n")
	if err := a.Discard(fileNamed(t, load(t, a), "scratch.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dirA, "scratch.txt")); !os.IsNotExist(err) {
		t.Fatalf("a's file still there: %v", err)
	}
	if got := readFile(t, dirB, "scratch.txt"); got != "b\n" {
		t.Fatalf("b's file: %q", got)
	}

	// Commits, loads and history rewrites of both at once stay apart.
	var wg sync.WaitGroup
	for i, r := range []*Repo{a, b, a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				if _, err := r.Load(true); err != nil {
					t.Error(err)
				}
			}
			if i < 2 {
				if err := r.DropCommit(load(t, r).Commits, 1); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	for _, dir := range []string{dirA, dirB} {
		if got := subjects(t, dir); len(got) != 2 || got[0] != "three" || got[1] != "one" {
			t.Fatalf("%s: %v", dir, got)
		}
	}
	if sa, sb := load(t, a), load(t, b); len(sa.Files) != 0 || len(sb.Files) != 1 {
		t.Fatalf("files: a %d, b %d", len(sa.Files), len(sb.Files))
	}
}
