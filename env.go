package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// importLoginPath gives the app the PATH of the user's login shell when it
// wasn't started from one, as from Finder or the Dock, whose PATH is
// launchd's /usr/bin:/bin:/usr/sbin:/sbin: without it, git would find no
// Homebrew tools, as gpg for signing commits, git-lfs, credential helpers,
// or the node that hooks run.
func importLoginPath() {
	if os.Getenv("TERM") != "" {
		return // a terminal's PATH is the user's already
	}
	if p := loginPath(); p != "" {
		os.Setenv("PATH", mergePaths(p, os.Getenv("PATH")))
	}
}

const pathMarker = "__LAZIERGIT_PATH__"

// loginPath asks the user's shell, as a login and interactive one so that
// it reads the files that set PATH, for its PATH: between markers, as the
// shell's startup files may print. A shell that takes too long is left.
func loginPath() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", `printf '`+pathMarker+`%s`+pathMarker+`' "$PATH"`)
	cmd.Stdin = nil
	// A program the shell's startup files leave running may hold its
	// output open: once the shell is done, or killed, wait no longer.
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	return betweenMarkers(string(out))
}

func betweenMarkers(out string) string {
	_, rest, ok := strings.Cut(out, pathMarker)
	if !ok {
		return ""
	}
	p, _, ok := strings.Cut(rest, pathMarker)
	if !ok {
		return ""
	}
	return p
}

// mergePaths is first's directories, then those of second it lacks.
func mergePaths(first, second string) string {
	seen := map[string]bool{}
	var out []string
	for _, d := range append(filepath.SplitList(first), filepath.SplitList(second)...) {
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}
