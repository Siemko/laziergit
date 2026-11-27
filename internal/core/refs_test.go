package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

func TestTags(t *testing.T) {
	r, dir := newRepo(t)
	if err := r.CreateTag("v1", "HEAD~1", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTag("v2", "HEAD", "Release two"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "cat-file", "-t", "v2"); got != "tag" {
		t.Fatalf("v2 is a %s, not annotated", got)
	}
	s := load(t, r)
	var v1 *models.Tag
	for _, tag := range s.Tags {
		if tag.Name == "v1" {
			v1 = tag
		}
	}
	if len(s.Tags) != 2 || v1 == nil {
		t.Fatalf("tags %+v", s.Tags)
	}
	commits, _, err := r.RefCommits(v1.FullRefName(), 10)
	if err != nil || len(commits) != 2 || commits[0].Name != "two" {
		t.Fatalf("v1's commits %v, %v", commits, err)
	}
	if err := r.CheckoutRef("refs/tags/v1"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "log", "-1", "--format=%s"); got != "two" {
		t.Fatalf("checked out %q", got)
	}
	if err := r.DeleteTag("v1"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "tag"); got != "v2" {
		t.Fatalf("tags left %q", got)
	}
}

// Names git would read as options are refused.
func TestRefsRejectOptions(t *testing.T) {
	r, dir := newRepo(t)
	for name, err := range map[string]error{
		"tag":            r.CreateTag("-d", "HEAD", ""),
		"tag ref":        r.CreateTag("v1", "--all", ""),
		"delete tag":     r.DeleteTag("-l"),
		"push tag":       r.PushTag("--mirror", "v1"),
		"checkout":       r.CheckoutRef("--orphan"),
		"reset":          r.ResetTo("--hard", ResetSoft),
		"fetch":          r.FetchRemote("--all"),
		"remote branch":  r.DeleteRemoteBranch(&models.RemoteBranch{Name: "x", RemoteName: "--receive-pack=evil"}),
		"submodule":      r.AddSubmodule("--upload-pack=evil", "sub"),
		"submodule path": r.AddSubmodule(dir, "../outside"),
		"worktree base":  r.AddWorktree("../wt", "--detach", ""),
	} {
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if got := git(t, dir, "tag"); got != "" {
		t.Fatalf("tags made: %q", got)
	}
}

// remoteRepo is a repository with a bare remote, origin, its main and a
// branch feature pushed, and fetched.
func remoteRepo(t *testing.T) (*Repo, string, string) {
	r, dir := newRepo(t)
	bare := filepath.Join(t.TempDir(), "origin.git")
	git(t, dir, "init", "-q", "--bare", bare)
	git(t, dir, "remote", "add", "origin", bare)
	git(t, dir, "branch", "feature", "HEAD~1")
	git(t, dir, "push", "-q", "origin", "main", "feature")
	git(t, dir, "branch", "-D", "feature")
	git(t, dir, "fetch", "-q", "origin")
	return r, dir, bare
}

func remoteBranch(t *testing.T, s *Snapshot, full string) *models.RemoteBranch {
	t.Helper()
	for _, remote := range s.Remotes {
		for _, b := range remote.Branches {
			if b.FullName() == full {
				return b
			}
		}
	}
	t.Fatalf("no remote branch %s in %+v", full, s.Remotes)
	return nil
}

func TestRemoteBranches(t *testing.T) {
	r, dir, bare := remoteRepo(t)
	s := load(t, r)
	feature := remoteBranch(t, s, "origin/feature")
	if err := r.CheckoutRemoteBranch(feature); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD@{upstream}"); got != "origin/feature" {
		t.Fatalf("upstream %q", got)
	}
	// Checked out again, the local branch is used.
	git(t, dir, "checkout", "-q", "main")
	if err := r.CheckoutRemoteBranch(feature); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteRemoteBranch(feature); err != nil {
		t.Fatal(err)
	}
	if got := git(t, bare, "branch", "--list", "feature"); got != "" {
		t.Fatalf("feature still on the remote: %q", got)
	}
	if err := r.FetchRemote("origin"); err != nil {
		t.Fatal(err)
	}
	if err := r.FetchRemote("nowhere"); err == nil {
		t.Fatal("fetched a remote the repository hasn't")
	}
	if err := r.PushTag("origin", "v9"); err == nil {
		t.Fatal("pushed a tag that isn't there")
	}
	if err := r.CreateTag("v1", "HEAD", ""); err != nil {
		t.Fatal(err)
	}
	if err := r.PushTag("origin", "v1"); err != nil {
		t.Fatal(err)
	}
	if got := git(t, bare, "tag"); got != "v1" {
		t.Fatalf("remote tags %q", got)
	}
}

func TestReflogReset(t *testing.T) {
	r, dir := newRepo(t)
	s := load(t, r)
	if len(s.Reflog) < 3 || !strings.HasPrefix(s.Reflog[0].Name, "commit") {
		t.Fatalf("reflog %v", s.Reflog)
	}
	two := git(t, dir, "rev-parse", "HEAD~1")
	if err := r.ResetTo(two, ResetSoft); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "diff", "--cached", "--name-only"); got != "three.txt" {
		t.Fatalf("staged after a soft reset: %q", got)
	}
	// A hard reset keeps an untracked file it would overwrite.
	three := git(t, dir, "rev-parse", "HEAD@{1}")
	git(t, dir, "reset", "-q", two)
	write(t, dir, "three.txt", "mine\n")
	if err := r.ResetTo(three, ResetHard); err == nil {
		t.Fatal("overwrote an untracked file")
	}
	os.Remove(filepath.Join(dir, "three.txt"))
	if err := r.ResetTo(three, ResetHard); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir)[0]; got != "three" {
		t.Fatalf("HEAD is %q", got)
	}
}

func TestWorktrees(t *testing.T) {
	r, dir := newRepo(t)
	if err := r.AddWorktree("../wt-fix", "main", "fix"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(dir), "wt-fix")
	t.Cleanup(func() { os.RemoveAll(path) })
	s := load(t, r)
	if len(s.Worktrees) != 2 {
		t.Fatalf("worktrees %+v", s.Worktrees)
	}
	var wt *models.Worktree
	for _, w := range s.Worktrees {
		if !w.IsMain {
			wt = w
		}
	}
	if wt == nil || wt.Branch != "fix" {
		t.Fatalf("worktree %+v", wt)
	}
	for _, w := range s.Worktrees {
		if w.IsMain {
			if err := r.RemoveWorktree(w, true); err == nil {
				t.Fatal("removed the main worktree")
			}
		}
	}
	write(t, path, "new.txt", "new\n")
	if err := r.RemoveWorktree(wt, false); !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("removed a worktree with changes: %v", err)
	}
	if err := r.RemoveWorktree(wt, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("worktree still there: %v", err)
	}
	// A folder with files in it isn't taken.
	os.MkdirAll(filepath.Join(dir, "..", "full"), 0o755)
	write(t, filepath.Join(dir, "..", "full"), "a.txt", "a")
	if err := r.AddWorktree("../full", "main", ""); err == nil {
		t.Fatal("made a worktree in a folder with files")
	}
}

func TestSubmodules(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "config", "--global", "protocol.file.allow", "always")
	lib := t.TempDir()
	git(t, lib, "init", "-q", "-b", "main")
	write(t, lib, "lib.txt", "lib\n")
	git(t, lib, "add", ".")
	git(t, lib, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "lib")
	if err := r.AddSubmodule(lib, "vendor/lib"); err != nil {
		t.Fatal(err)
	}
	s := load(t, r)
	if len(s.Submodules) != 1 {
		t.Fatalf("submodules %+v", s.Submodules)
	}
	sm := s.Submodules[0]
	if sm.Path != "vendor/lib" || !sm.Initialized() || sm.Head == "" {
		t.Fatalf("submodule %+v", sm)
	}
	git(t, dir, "commit", "-q", "-m", "add lib")
	// Deinitialized, an update checks it out again.
	git(t, dir, "submodule", "deinit", "-q", "-f", "vendor/lib")
	s = load(t, r)
	if s.Submodules[0].Initialized() {
		t.Fatalf("still initialized: %+v", s.Submodules[0])
	}
	if err := r.UpdateSubmodule(s.Submodules[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/lib/lib.txt")); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveSubmodule(load(t, r).Submodules[0]); err != nil {
		t.Fatal(err)
	}
	if s := load(t, r); len(s.Submodules) != 0 {
		t.Fatalf("submodules left %+v", s.Submodules)
	}
}

// The reflog read, its latest entries alone, is lazygit's reflog loader's.
func TestReflogAsLazygits(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "reset", "-q", "--hard", "HEAD~1")
	leave, err := r.enter()
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := r.git.Loaders.ReflogCommitLoader.GetReflogCommits(r.hashPool, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.reflog(3)
	leave()
	if err != nil || len(got) != 3 || len(want) < 3 {
		t.Fatalf("%d entries, want 3 of %d: %v", len(got), len(want), err)
	}
	for i, c := range got {
		w := want[i]
		if c.Hash() != w.Hash() || c.Name != w.Name || c.UnixTimestamp != w.UnixTimestamp || c.Status != w.Status ||
			strings.Join(c.Parents(), " ") != strings.Join(w.Parents(), " ") {
			t.Fatalf("entry %d: %+v, lazygit's %+v", i, c, w)
		}
	}
}
