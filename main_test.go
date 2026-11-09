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
