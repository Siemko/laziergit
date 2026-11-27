package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBetweenMarkers(t *testing.T) {
	out := "Welcome to zsh!\n" + pathMarker + "/opt/homebrew/bin:/usr/bin" + pathMarker + "\nbye"
	if got := betweenMarkers(out); got != "/opt/homebrew/bin:/usr/bin" {
		t.Fatalf("got %q", got)
	}
	if got := betweenMarkers("no markers"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestMergePaths(t *testing.T) {
	got := mergePaths("/opt/homebrew/bin:/usr/bin", "/usr/bin:/bin::/usr/sbin")
	if got != "/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin" {
		t.Fatalf("got %q", got)
	}
}

func TestRepoArgs(t *testing.T) {
	wd := t.TempDir()
	os.Mkdir(filepath.Join(wd, "repo"), 0o755)
	got := repoArgs([]string{"-psn_0_123", "repo", "missing", "/"}, wd)
	if !slices.Equal(got, []string{filepath.Join(wd, "repo"), "/"}) {
		t.Fatalf("got %v", got)
	}
}

// A repository's copies go with its last window, rather than wait for a
// repository made at its path later.
func TestDropCopies(t *testing.T) {
	m := &manager{}
	m.copiesOf("/repo/.git")
	m.dropCopies()
	if len(m.copies) != 0 {
		t.Fatalf("copies kept: %v", m.copies)
	}
}

func TestInCellar(t *testing.T) {
	for path, want := range map[string]bool{
		"/opt/homebrew/Cellar/laziergit/0.6.0/laziergit.app/Contents/MacOS/laziergit": true,
		"/usr/local/Cellar/laziergit/0.6.0/laziergit.app/Contents/MacOS/laziergit":    true,
		"/Applications/laziergit.app/Contents/MacOS/laziergit":                        false,
	} {
		if got := inCellar(path); got != want {
			t.Errorf("%s: %v", path, got)
		}
	}
}
