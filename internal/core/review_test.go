package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests below each guard against a case of the review before v0.1.0.

func TestPathsAreLiteral(t *testing.T) {
	r, dir := newRepo(t)
	name := ":(glob)*"
	write(t, dir, name, "x\n")
	git(t, dir, "add", "--", name)
	git(t, dir, "commit", "-q", "-m", "magic name")
	write(t, dir, name, "changed\n")
	write(t, dir, "one.txt", "one edited\n")
	// git reads the name as the file's, not as a pattern of every file.
	if err := r.Discard(fileNamed(t, load(t, r), name)); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, name); got != "x\n" {
		t.Fatalf("the file itself: %q", got)
	}
	if got := readFile(t, dir, "one.txt"); got != "one edited\n" {
		t.Fatalf("one.txt lost its edit: %q", got)
	}
}

func TestOptionLikeBranchRefused(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "update-ref", "refs/heads/--force", "HEAD")
	write(t, dir, "one.txt", "edited\n")
	if err := r.Checkout("--force"); err == nil {
		t.Fatal("checked out --force")
	}
	if got := readFile(t, dir, "one.txt"); got != "edited\n" {
		t.Fatalf("edit lost: %q", got)
	}
}

func TestPartialStageOfUnpatchableNameRefused(t *testing.T) {
	r, dir := newRepo(t)
	name := "target.txt\tother"
	write(t, dir, "target.txt", "a\nb\n")
	write(t, dir, name, "a\nb\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "tab")
	write(t, dir, name, "A\nB\n")
	f := fileNamed(t, load(t, r), name)
	diff, _ := r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "+A"), StageLines); err == nil {
		t.Fatal("staged part of a file named with a tab")
	}
	if staged := git(t, dir, "diff", "--cached", "--name-only"); staged != "" {
		t.Fatalf("staged %q", staged)
	}
}

func TestApplyLinesRefusesStaleDiff(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "new.txt", "seen\n")
	f := fileNamed(t, load(t, r), "new.txt")
	diff, _ := r.FileDiff(f, false)
	write(t, dir, "new.txt", "seen\nunseen\n") // edited since the diff showed
	if err := r.ApplyLines(f, diff, linesMatching(diff, "seen"), DiscardLines); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "new.txt"); got != "seen\nunseen\n" {
		t.Fatalf("file %q", got)
	}
}

func TestDiscardAllLinesOfAddedFileKeepsIt(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "added.txt", "staged\n")
	git(t, dir, "add", "added.txt")
	write(t, dir, "added.txt", "staged\nunstaged\n") // AM
	f := fileNamed(t, load(t, r), "added.txt")
	diff, _ := r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "unstaged"), DiscardLines); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, "added.txt"); got != "staged\n" {
		t.Fatalf("work tree %q", got)
	}
	if got := git(t, dir, "show", ":added.txt"); got != "staged" {
		t.Fatalf("index %q", got)
	}
}

func TestUndoKeepsOtherStashesAndUntrackedFiles(t *testing.T) {
	r, dir := newRepo(t)
	// A stash of the user's own, and only untracked files around: the
	// undo must neither pop the stash nor lose the files.
	write(t, dir, "one.txt", "mine\n")
	git(t, dir, "stash", "push", "-q", "-m", "keep me")
	if err := r.DropCommit(load(t, r).Commits, 1); err != nil { // drops "two"
		t.Fatal(err)
	}
	write(t, dir, "scratch.txt", "untracked\n")
	undo(t, r, false)
	if list := git(t, dir, "stash", "list"); !strings.Contains(list, "keep me") {
		t.Fatalf("the user's stash is gone: %q", list)
	}
	if got := readFile(t, dir, "scratch.txt"); got != "untracked\n" {
		t.Fatalf("scratch.txt %q", got)
	}

	// An untracked file where the target has one is refused, not lost.
	if err := r.DropCommit(load(t, r).Commits, 1); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "two.txt", "precious\n") // untracked now, tracked in the target
	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyUndo(s, false); err == nil || !strings.Contains(err.Error(), "two.txt") {
		t.Fatalf("undo over an untracked file: %v", err)
	}
	if got := readFile(t, dir, "two.txt"); got != "precious\n" {
		t.Fatalf("two.txt %q", got)
	}
}

func TestMovedRepositoryFailsInsteadOfActingElsewhere(t *testing.T) {
	a, dirA := newRepo(t)
	_, dirB := newRepo(t) // current now
	write(t, dirA, "notes.txt", "a\n")
	write(t, dirB, "notes.txt", "b\n")
	f := fileNamed(t, load(t, a), "notes.txt")
	moved := dirA + "-moved"
	if err := os.Rename(dirA, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Rename(moved, dirA) })
	if err := a.Discard(f); err == nil {
		t.Fatal("discard in a moved repository succeeded")
	}
	if got := readFile(t, dirB, "notes.txt"); got != "b\n" {
		t.Fatalf("b's file: %q", got)
	}
}

func TestConflictedSymlinkIsNotText(t *testing.T) {
	r, dir := newRepo(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	os.WriteFile(outside, []byte("<<<<<<< a\nx\n=======\ny\n>>>>>>> b\n"), 0o644)
	git(t, dir, "checkout", "-q", "-b", "side")
	os.Symlink(outside, filepath.Join(dir, "link"))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "side link")
	git(t, dir, "checkout", "-q", "main")
	os.Symlink(filepath.Join(dir, "one.txt"), filepath.Join(dir, "link"))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "main link")
	if err := r.Merge("side"); !IsConflict(err) {
		t.Fatalf("merge: %v", err)
	}
	f := fileNamed(t, load(t, r), "link")
	if _, err := r.ReadConflicted(f.Path); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("read: %v", err)
	}
	if _, err := r.ResolveConflict(f, "", "resolved\n"); !errors.Is(err, ErrNotRegular) {
		t.Fatalf("resolve: %v", err)
	}
	if b, _ := os.ReadFile(outside); !strings.HasPrefix(string(b), "<<<<<<<") {
		t.Fatalf("outside file written: %q", b)
	}
}

func TestResolveConflictRefusesExternalEdit(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side", "HEAD~1")
	write(t, dir, "two.txt", "side\n")
	git(t, dir, "commit", "-q", "-am", "side")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "two.txt", "main\n")
	git(t, dir, "commit", "-q", "-am", "main")
	if err := r.Merge("side"); !IsConflict(err) {
		t.Fatalf("merge: %v", err)
	}
	f := fileNamed(t, load(t, r), "two.txt")
	content, _ := r.ReadConflicted(f.Path)
	edited := content + "added in an editor\n"
	write(t, dir, "two.txt", edited)
	if _, err := r.ResolveConflict(f, content, ParseConflicts(content, 0).Resolve(0, PickOurs)); !errors.Is(err, ErrStale) {
		t.Fatalf("resolve: %v", err)
	}
	if got := readFile(t, dir, "two.txt"); got != edited {
		t.Fatalf("edit lost: %q", got)
	}
	// Staging a conflicted file would stage its markers.
	if err := r.ToggleStaged(f); err == nil {
		t.Fatal("staged a conflicted file")
	}
	if err := r.ToggleAllStaged(load(t, r).Files); err == nil {
		t.Fatal("staged all with a conflicted file")
	}
}

func TestOpenAfterSeparateGitDir(t *testing.T) {
	_, _ = newRepo(t)
	parent := t.TempDir()
	work, gitDir := filepath.Join(parent, "work"), filepath.Join(parent, "gitdir")
	git(t, parent, "init", "-q", "-b", "main", "--separate-git-dir", gitDir, work)
	git(t, work, "commit", "-q", "--allow-empty", "-m", "separate")
	a, err := Open(work, Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, dirB := newRepo(t)
	if real(t, b.Path) != real(t, dirB) {
		t.Fatalf("opened %s for %s", b.Path, dirB)
	}
}

func real(t *testing.T, p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAmendWithMessageAndDeleteUnmerged(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "one.txt", "amended\n")
	git(t, dir, "add", ".")
	if err := r.Commit("three, amended", "with a body", true); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "log", "-1", "--format=%s|%b"); got != "three, amended|with a body" {
		t.Fatalf("message %q", got)
	}
	if got := git(t, dir, "show", "HEAD:one.txt"); got != "amended" {
		t.Fatalf("content %q", got)
	}
	git(t, dir, "checkout", "-q", "-b", "wip")
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "wip")
	git(t, dir, "checkout", "-q", "main")
	if err := r.DeleteBranch("wip", false); !errors.Is(err, ErrNotMerged) {
		t.Fatalf("delete: %v", err)
	}
	if err := r.DeleteBranch("wip", true); err != nil {
		t.Fatal(err)
	}
}

func TestDropMergeCommit(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side", "HEAD~1")
	write(t, dir, "side.txt", "side\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "side")
	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "merge", "-q", "--no-ff", "-m", "merge side", "side")
	s := load(t, r)
	if err := r.DropCommit(s.Commits, 0); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir)[0]; got != "three" {
		t.Fatalf("HEAD %q", got)
	}
	if err := r.FixupCommit(load(t, r).Commits, 0); err != nil {
		t.Fatal(err)
	}
}

// Round 2 of the review.

func TestUndoRefusesToOverwriteWhatGitDoesntTrack(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		// The target has a file where an untracked directory is.
		"directory": func(t *testing.T, dir string) {
			os.MkdirAll(filepath.Join(dir, "two.txt"), 0o755)
			write(t, dir, "two.txt/precious.txt", "precious\n")
		},
		// The target tracks a file the work tree ignores now.
		"ignored": func(t *testing.T, dir string) {
			write(t, dir, ".gitignore", "two.txt\n")
			git(t, dir, "add", ".gitignore")
			git(t, dir, "commit", "-q", "-m", "ignore two")
			write(t, dir, "two.txt", "precious\n")
		},
		// Untracked files the status hides by config.
		"hidden by config": func(t *testing.T, dir string) {
			git(t, dir, "config", "status.showUntrackedFiles", "no")
			write(t, dir, "two.txt", "precious\n")
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			r, dir := newRepo(t)
			if err := r.DropCommit(load(t, r).Commits, 1); err != nil { // drops "two"
				t.Fatal(err)
			}
			setup(t, dir)
			s, err := r.FindUndo(false)
			if err != nil {
				t.Fatal(err)
			}
			if s.Kind == UndoCommit {
				// "ignored" committed after the drop: undo that first.
				undo(t, r, false)
				if s, err = r.FindUndo(false); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.ApplyUndo(s, false); err == nil {
				t.Fatalf("undo %+v overwrote what git doesn't track", s)
			}
			var precious string
			for _, p := range []string{"two.txt", "two.txt/precious.txt"} {
				if b, err := os.ReadFile(filepath.Join(dir, p)); err == nil {
					precious = string(b)
				}
			}
			if precious != "precious\n" {
				t.Fatalf("lost: %q", precious)
			}
		})
	}
}

func TestUndoCheckoutToOptionLikeBranchRefused(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "update-ref", "refs/heads/--force", "HEAD")
	git(t, dir, "symbolic-ref", "HEAD", "refs/heads/--force")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "one.txt", "edited\n")
	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyUndo(s, false); err == nil {
		t.Fatalf("undid %+v", s)
	}
	if got := readFile(t, dir, "one.txt"); got != "edited\n" {
		t.Fatalf("edit lost: %q", got)
	}
}

func TestDiscardAllLinesOfUnpatchableNameAffectsOnlyIt(t *testing.T) {
	r, dir := newRepo(t)
	name := "target.txt\tother"
	write(t, dir, "target.txt", "a\n")
	write(t, dir, name, "a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "tab")
	write(t, dir, "target.txt", "b\n")
	write(t, dir, name, "b\n")
	f := fileNamed(t, load(t, r), name)
	diff, _ := r.FileDiff(f, false)
	if err := r.ApplyLines(f, diff, linesMatching(diff, "a", "b"), DiscardLines); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, "target.txt"); got != "b\n" {
		t.Fatalf("target.txt changed: %q", got)
	}
	if got := readFile(t, dir, name); got != "a\n" {
		t.Fatalf("the file itself: %q", got)
	}
}

func TestNestedRepositoryIsNotDiscarded(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "vendored")
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	f := fileNamed(t, load(t, r), "vendored")
	if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Fatalf("nested repository gone: %v", err)
	}
	if err := r.DeleteNestedRepository(f); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("still there: %v", err)
	}
}

func TestHooksKeepTheirPatterns(t *testing.T) {
	r, dir := newRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	out := filepath.Join(t.TempDir(), "seen")
	os.WriteFile(hook, []byte("#!/bin/sh\ngit diff --cached --name-only -- '*.txt' > "+out+"\n"), 0o755)
	write(t, dir, "four.txt", "four\n")
	git(t, dir, "add", ".")
	if err := r.Commit("four", "", false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); strings.TrimSpace(string(b)) != "four.txt" {
		t.Fatalf("the hook's pattern matched %q", b)
	}
}

// Round 3 of the review.

func TestBracketedPathsAreTheirOwn(t *testing.T) {
	r, dir := newRepo(t)
	os.MkdirAll(filepath.Join(dir, "app", "[slug]"), 0o755)
	os.MkdirAll(filepath.Join(dir, "app", "s"), 0o755)
	write(t, dir, "app/[slug]/page.tsx", "slug\n")
	write(t, dir, "app/s/page.tsx", "s\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "routes")
	write(t, dir, "app/[slug]/page.tsx", "slug edited\n")
	write(t, dir, "app/s/page.tsx", "s edited\n")
	f := fileNamed(t, load(t, r), "app/[slug]/page.tsx")
	diff, err := r.FileDiff(f, false)
	if err != nil || !strings.Contains(diff, "+slug edited") || strings.Contains(diff, "s edited") {
		t.Fatalf("diff %v:\n%s", err, diff)
	}
	if err := r.ToggleStaged(f); err != nil {
		t.Fatal(err)
	}
	if staged := git(t, dir, "diff", "--cached", "--name-only"); staged != "app/[slug]/page.tsx" {
		t.Fatalf("staged %q", staged)
	}
	if err := r.ToggleStaged(fileNamed(t, load(t, r), "app/[slug]/page.tsx")); err != nil {
		t.Fatal(err)
	}
	if err := r.Discard(fileNamed(t, load(t, r), "app/[slug]/page.tsx")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, "app/s/page.tsx"); got != "s edited\n" {
		t.Fatalf("the other route lost its edit: %q", got)
	}
	if got := readFile(t, dir, "app/[slug]/page.tsx"); got != "slug\n" {
		t.Fatalf("not discarded: %q", got)
	}
}

func TestRenameFromPatternNameAffectsOnlyIt(t *testing.T) {
	r, dir := newRepo(t)
	name := ":(glob)*"
	write(t, dir, name, "x\n")
	git(t, dir, "add", "--", name)
	git(t, dir, "commit", "-q", "-m", "magic name")
	git(t, dir, "mv", "--", name, "normal.txt")
	write(t, dir, "one.txt", "one edited\n")
	write(t, dir, "two.txt", "two staged\n")
	git(t, dir, "add", "two.txt")
	f := fileNamed(t, load(t, r), "normal.txt")
	if f.PreviousPath != name {
		t.Fatalf("not a rename: %+v", f)
	}
	// Unstaging the rename leaves the other staged change staged.
	if err := r.ToggleStaged(f); err != nil {
		t.Fatal(err)
	}
	if staged := git(t, dir, "diff", "--cached", "--name-only"); staged != "two.txt" && !strings.Contains(staged, "two.txt") {
		t.Fatalf("staged after unstaging the rename: %q", staged)
	}
	// Discarding it (staged again) leaves one.txt's edit.
	git(t, dir, "add", "-A", "--", "normal.txt", name)
	if err := r.Discard(fileNamed(t, load(t, r), "normal.txt")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, dir, "one.txt"); got != "one edited\n" {
		t.Fatalf("one.txt lost its edit: %q", got)
	}
	if got := git(t, dir, "diff", "--cached", "--name-only", "--", "two.txt"); got != "two.txt" {
		t.Fatalf("two.txt not staged any more: %q", got)
	}
}

func TestUndoWritesIntoUntrackedDirectory(t *testing.T) {
	r, dir := newRepo(t)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/guide.md", "guide\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "docs")
	git(t, dir, "rm", "-q", "-r", "docs")
	git(t, dir, "commit", "-q", "-m", "no docs")
	// Finder leaves a file in the folder: the undo writes guide.md beside it.
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/.DS_Store", "finder\n")
	undo(t, r, false) // the commit "no docs", softly
	git(t, dir, "reset", "-q", "--hard", "HEAD")
	if got := readFile(t, dir, "docs/guide.md"); got != "guide\n" {
		t.Fatalf("guide.md %q", got)
	}
	if got := readFile(t, dir, "docs/.DS_Store"); got != "finder\n" {
		t.Fatalf(".DS_Store %q", got)
	}
}

func TestUndoHardIntoUntrackedDirectory(t *testing.T) {
	r, dir := newRepo(t)
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/guide.md", "guide\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "docs")
	// Drop "docs" by a rebase, whose undo is a hard reset to it.
	if err := r.DropCommit(load(t, r).Commits, 0); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/.DS_Store", "finder\n")
	if s := undo(t, r, false); s.Kind != UndoRebase {
		t.Fatalf("undid %+v", s)
	}
	if got := readFile(t, dir, "docs/guide.md"); got != "guide\n" {
		t.Fatalf("guide.md %q", got)
	}
	// A file of its own at a path the target writes is refused.
	if err := r.DropCommit(load(t, r).Commits, 0); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "docs/guide.md", "my own guide\n")
	s, _ := r.FindUndo(false)
	if err := r.ApplyUndo(s, false); err == nil {
		t.Fatal("overwrote an untracked file in an untracked directory")
	}
	if got := readFile(t, dir, "docs/guide.md"); got != "my own guide\n" {
		t.Fatalf("lost %q", got)
	}
}

func TestDeleteBranchMergedToHeadNotUpstream(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "branch", "base", "HEAD~2")
	git(t, dir, "branch", "old", "HEAD")
	git(t, dir, "branch", "--set-upstream-to=base", "old")
	if err := r.DeleteBranch("old", false); !errors.Is(err, ErrNotMerged) {
		t.Fatalf("delete: %v", err)
	}
}

// Round 4 of the review.

func TestPathCommandHooksKeepTheirPatterns(t *testing.T) {
	r, dir := newRepo(t)
	out := filepath.Join(t.TempDir(), "seen")
	for _, hook := range []string{"post-checkout", "post-index-change"} {
		os.WriteFile(filepath.Join(dir, ".git", "hooks", hook), []byte("#!/bin/sh\necho \"${GIT_LITERAL_PATHSPECS:-unset} $(git ls-files -- '*.txt' | tr '\\n' ' ')\" >> "+out+"\n"), 0o755)
	}
	write(t, dir, "one.txt", "edited\n")
	if err := r.ToggleStaged(fileNamed(t, load(t, r), "one.txt")); err != nil { // post-index-change
		t.Fatal(err)
	}
	if err := r.Discard(fileNamed(t, load(t, r), "one.txt")); err != nil { // post-checkout
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("no hook ran")
	}
	for _, l := range lines {
		if !strings.HasPrefix(l, "unset ") || !strings.Contains(l, "one.txt") {
			t.Fatalf("a hook saw %q", l)
		}
	}
}

func TestMarkResolvedRefusesFileTooLargeToCheck(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side", "HEAD~1")
	write(t, dir, "two.txt", "side\n")
	git(t, dir, "commit", "-q", "-am", "side")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "two.txt", "main\n")
	git(t, dir, "commit", "-q", "-am", "main")
	if err := r.Merge("side"); !IsConflict(err) {
		t.Fatalf("merge: %v", err)
	}
	f := fileNamed(t, load(t, r), "two.txt")
	big, _ := os.OpenFile(filepath.Join(dir, "two.txt"), os.O_APPEND|os.O_WRONLY, 0)
	big.Truncate(maxConflictedSize + 1)
	big.Close()
	if err := r.MarkResolved(f); err == nil {
		t.Fatal("staged a conflicted file too large to check")
	}
	if u := git(t, dir, "ls-files", "-u", "--", "two.txt"); u == "" {
		t.Fatal("no longer unmerged")
	}
}

func TestContinueStoppingAgainIsAConflict(t *testing.T) {
	r, dir := newRepo(t)
	// Two commits of a branch, each conflicting with main.
	git(t, dir, "checkout", "-q", "-b", "side", "HEAD~2")
	write(t, dir, "two.txt", "side two\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "side two")
	write(t, dir, "three.txt", "side three\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "side three")
	if err := r.RebaseOnto("main"); !IsConflict(err) {
		t.Fatalf("rebase: %v", err)
	}
	f := fileNamed(t, load(t, r), "two.txt")
	content, _ := r.ReadConflicted(f.Path)
	if _, err := r.ResolveConflict(f, content, ParseConflicts(content, 0).Resolve(0, PickTheirs)); err != nil {
		t.Fatal(err)
	}
	// The continue applies the first and stops on the second: a conflict,
	// though the state is rebasing before and after, in any language.
	t.Setenv("LC_ALL", "pl_PL.UTF-8")
	t.Setenv("LANG", "pl_PL.UTF-8")
	err := r.ContinueOperation(load(t, r).State)
	var ce conflictError
	if !errors.As(err, &ce) {
		t.Fatalf("continue: %v", err)
	}
	// A pull refused during the rebase is an error, not a conflict.
	if err := r.Pull(); errors.As(err, &ce) {
		t.Fatalf("pull during a rebase taken for a conflict: %v", err)
	}
}

func TestDeleteBranchOtherRefusalIsGitsError(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "branch", "elsewhere")
	git(t, dir, "worktree", "add", "-q", filepath.Join(t.TempDir(), "wt"), "elsewhere")
	err := r.DeleteBranch("elsewhere", false)
	if err == nil || errors.Is(err, ErrNotMerged) {
		t.Fatalf("delete: %v", err)
	}
}

func TestUndoRefusesFileByAnotherCase(t *testing.T) {
	r, dir := newRepo(t)
	probe := filepath.Join(dir, "PROBE")
	os.WriteFile(probe, nil, 0o644)
	_, err := os.Stat(filepath.Join(dir, "probe"))
	os.Remove(probe)
	if err != nil {
		t.Skip("the disk tells case apart")
	}
	os.MkdirAll(filepath.Join(dir, "Docs"), 0o755)
	write(t, dir, "Docs/A.md", "a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "docs")
	if err := r.DropCommit(load(t, r).Commits, 0); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/a.md", "mine\n")
	s, _ := r.FindUndo(false)
	if err := r.ApplyUndo(s, false); err == nil {
		t.Fatal("overwrote a file named by another case")
	}
	if got := readFile(t, dir, "docs/a.md"); got != "mine\n" {
		t.Fatalf("lost %q", got)
	}
}
