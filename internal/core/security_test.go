package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

// A checkout of a branch deleted since names, for git, the directory of
// the same name: it must fail rather than throw away that directory's edits.
func TestCheckoutOfDeletedBranchKeepsDirectoryOfItsName(t *testing.T) {
	r, dir := newRepo(t)
	os.Mkdir(filepath.Join(dir, "docs"), 0o755)
	write(t, dir, "docs/a.md", "a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "docs")
	if err := r.NewBranch("docs", "HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := r.Checkout("main"); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "branch", "-D", "docs")
	write(t, dir, "docs/a.md", "edited\n")

	if err := r.Checkout("docs"); err == nil {
		t.Fatal("checked out a deleted branch")
	}
	if got := readFile(t, dir, "docs/a.md"); got != "edited\n" {
		t.Fatalf("after checkout: %q", got)
	}

	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != UndoCheckout || s.From != "docs" {
		t.Fatalf("undo step %+v", s)
	}
	if err := r.ApplyUndo(s, false); err == nil {
		t.Fatal("undid to a deleted branch")
	}
	if got := readFile(t, dir, "docs/a.md"); got != "edited\n" {
		t.Fatalf("after undo: %q", got)
	}
}

// A directory turned into a symlink between the status and the discard
// leads out of the work tree: what it leads to isn't the repository's.
func TestDiscardDoesntFollowSymlinkedDirectory(t *testing.T) {
	r, dir := newRepo(t)
	os.Mkdir(filepath.Join(dir, "folder"), 0o755)
	write(t, dir, "folder/victim", "mine\n")
	f := fileNamed(t, load(t, r), "folder/victim")

	outside := t.TempDir()
	write(t, outside, "victim", "not the repository's\n")
	os.RemoveAll(filepath.Join(dir, "folder"))
	if err := os.Symlink(outside, filepath.Join(dir, "folder")); err != nil {
		t.Fatal(err)
	}
	if err := r.Discard(f); err == nil {
		t.Fatal("discarded through a symlink")
	}
	if got := readFile(t, outside, "victim"); got != "not the repository's\n" {
		t.Fatalf("outside file: %q", got)
	}
}

// With status.showUntrackedFiles=normal an untracked directory is one
// entry: a repository deeper in it must not go with it unasked.
func TestDiscardOfDirectoryHoldingRepositoryRefused(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "config", "status.showUntrackedFiles", "normal")
	nested := filepath.Join(dir, "vendor", "lib")
	os.MkdirAll(nested, 0o755)
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	f := fileNamed(t, load(t, r), "vendor")

	if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Fatalf("nested repository gone: %v", err)
	}
}

// An untracked symlink is discarded as itself, wherever it leads: what it
// leads to stays.
func TestDiscardOfUntrackedSymlinkRemovesOnlyIt(t *testing.T) {
	r, dir := newRepo(t)
	outside := t.TempDir()
	write(t, outside, "kept", "kept\n")
	for name, target := range map[string]string{"out": outside, "here": ".", "file": "one.txt", "broken": "nowhere"} {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	s := load(t, r)
	for _, name := range []string{"out", "here", "file", "broken"} {
		if err := r.Discard(fileNamed(t, s, name)); err != nil {
			t.Fatalf("discard %s: %v", name, err)
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still there: %v", name, err)
		}
	}
	if got := readFile(t, outside, "kept"); got != "kept\n" {
		t.Fatalf("outside: %q", got)
	}
	if got := readFile(t, dir, "one.txt"); got != "one\n" {
		t.Fatalf("one.txt: %q", got)
	}
}

// A disk that doesn't tell case apart, as macOS's by default, finds a
// repository's .git under another spelling, and so does git.
func TestDiscardOfDirectoryHoldingRepositorySpelledOtherwise(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "vendor", "lib")
	os.MkdirAll(nested, 0o755)
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	if err := os.Rename(filepath.Join(nested, ".git"), filepath.Join(nested, ".GIT")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Skip("the disk tells case apart")
	}
	git(t, dir, "config", "status.showUntrackedFiles", "normal")
	if err := r.Discard(fileNamed(t, load(t, r), "vendor")); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".GIT")); err != nil {
		t.Fatalf("nested repository gone: %v", err)
	}
}

// A file where a directory was shown isn't the directory the user chose
// to throw away.
func TestDiscardOfDirectoryTurnedFileRefused(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "config", "status.showUntrackedFiles", "normal")
	os.Mkdir(filepath.Join(dir, "vendor"), 0o755)
	write(t, dir, "vendor/a", "a\n")
	f := fileNamed(t, load(t, r), "vendor")
	os.RemoveAll(filepath.Join(dir, "vendor"))
	write(t, dir, "vendor", "precious\n")
	if err := r.Discard(f); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "vendor"); got != "precious\n" {
		t.Fatalf("vendor: %q", got)
	}
}

// A repository added to the index, as a gitlink, is refused as one before
// anything is done: the index keeps it too.
func TestDiscardOfStagedRepositoryRefusedUntouched(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "sub")
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	git(t, dir, "add", "sub")
	if err := r.Discard(fileNamed(t, load(t, r), "sub")); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "A  sub" {
		t.Fatalf("status: %q", st)
	}
}

// nestedWithIndex makes a repository at sub, with a file in it, then sets
// the index entries of sub to what info says (git update-index
// --index-info), the commit of sub standing for H.
func nestedWithIndex(t *testing.T, info string) (*Repo, string) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "sub")
	git(t, dir, "init", "-q", nested)
	write(t, nested, "precious", "precious\n")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "-q", "-m", "its own history")
	h := git(t, nested, "rev-parse", "HEAD")
	cmd := exec.Command("git", "update-index", "--index-info")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.ReplaceAll(info, "H", h))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-index: %v\n%s", err, out)
	}
	return r, dir
}

// A conflict of a repository's commit, deleted by us (DU), is discarded
// by git rm, which would take the repository's files: refused as one.
func TestDiscardOfDeletedByUsRepositoryRefused(t *testing.T) {
	r, dir := nestedWithIndex(t, "160000 H 1\tsub\n160000 H 3\tsub\n")
	f := fileNamed(t, load(t, r), "sub")
	if f.ShortStatus != "DU" {
		t.Fatalf("status %q", f.ShortStatus)
	}
	if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "sub/precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// A conflict of a repository's commit, added by both (AA), is discarded by
// keeping ours, which removes nothing: it isn't refused.
func TestDiscardOfBothAddedRepositoryKeepsOurs(t *testing.T) {
	r, dir := nestedWithIndex(t, "160000 H 2\tsub\n160000 H 3\tsub\n")
	f := fileNamed(t, load(t, r), "sub")
	if f.ShortStatus != "AA" {
		t.Fatalf("status %q", f.ShortStatus)
	}
	if err := r.Discard(f); err != nil {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "sub/precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// A conflict of files added by both, whose path holds a repository since,
// isn't discarded by checking ours out over it: refused as a repository.
func TestDiscardOfBothAddedFileTurnedRepositoryRefused(t *testing.T) {
	r, dir := newRepo(t)
	for _, branch := range []string{"side", "main"} {
		if branch == "side" {
			git(t, dir, "checkout", "-q", "-b", "side")
		} else {
			git(t, dir, "checkout", "-q", "main")
		}
		write(t, dir, "new.txt", branch+"\n")
		git(t, dir, "add", "new.txt")
		git(t, dir, "commit", "-q", "-m", branch)
	}
	cmd := exec.Command("git", "merge", "side")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("merge didn't conflict")
	}
	os.Remove(filepath.Join(dir, "new.txt"))
	nested := filepath.Join(dir, "new.txt")
	git(t, dir, "init", "-q", nested)
	write(t, nested, "precious", "precious\n")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "-q", "-m", "its own history")
	f := fileNamed(t, load(t, r), "new.txt")
	if f.ShortStatus != "AA" {
		t.Fatalf("status %q", f.ShortStatus)
	}
	if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, nested, "precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// A file made at a rename's old path isn't the old file, whose return the
// discard brings: refused, rather than thrown away.
func TestDiscardOfRenameKeepsNewFileAtOldPath(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "mv", "one.txt", "moved.txt")
	write(t, dir, "one.txt", "precious new file\n")
	var rename *models.File
	for _, f := range load(t, r).Files {
		if f.IsRename() {
			rename = f
		}
	}
	if rename == nil {
		t.Skip("git shows no rename")
	}
	if err := r.Discard(rename); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "one.txt"); got != "precious new file\n" {
		t.Fatalf("one.txt: %q", got)
	}
}

// What is at a rename's old path since, as the disk has it, is new: a
// directory of files, listed each for itself as untracked, or a file
// ignored, which git status doesn't list, isn't overwritten by the old
// file's return.
func TestDiscardOfRenameKeepsWhatIsAtOldPath(t *testing.T) {
	for name, make := range map[string]func(t *testing.T, dir string) string{
		"directory": func(t *testing.T, dir string) string {
			os.Mkdir(filepath.Join(dir, "one.txt"), 0o755)
			write(t, dir, "one.txt/precious", "precious\n")
			return "one.txt/precious"
		},
		"ignored": func(t *testing.T, dir string) string {
			write(t, dir, ".git/info/exclude", "one.txt\n")
			write(t, dir, "one.txt", "precious\n")
			return "one.txt"
		},
	} {
		t.Run(name, func(t *testing.T) {
			r, dir := newRepo(t)
			git(t, dir, "mv", "one.txt", "moved.txt")
			kept := make(t, dir)
			var rename *models.File
			for _, f := range load(t, r).Files {
				if f.IsRename() {
					rename = f
				}
			}
			if rename == nil {
				t.Fatal("no rename")
			}
			if err := r.Discard(rename); !errors.Is(err, ErrStale) {
				t.Fatalf("discard: %v", err)
			}
			if got := readFile(t, dir, kept); got != "precious\n" {
				t.Fatalf("%s: %q", kept, got)
			}
		})
	}
}

// A file deleted, with a directory of new files made at its path since,
// is brought back only where nothing is: the files stay. With nothing
// there, it is brought back.
func TestDiscardOfDeletionKeepsWhatIsAtItsPath(t *testing.T) {
	r, dir := newRepo(t)
	os.Remove(filepath.Join(dir, "one.txt"))
	os.Mkdir(filepath.Join(dir, "one.txt"), 0o755)
	write(t, dir, "one.txt/precious", "precious\n")
	os.Remove(filepath.Join(dir, "two.txt"))
	s := load(t, r)
	if err := r.Discard(fileNamed(t, s, "one.txt")); !errors.Is(err, ErrStale) {
		t.Fatalf("discard one.txt: %v", err)
	}
	if got := readFile(t, dir, "one.txt/precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
	if err := r.Discard(fileNamed(t, s, "two.txt")); err != nil {
		t.Fatalf("discard two.txt: %v", err)
	}
	if got := readFile(t, dir, "two.txt"); got != "two\n" {
		t.Fatalf("two.txt: %q", got)
	}
}

// git checkout, bringing a file back, takes away a directory at its path:
// a repository there, as a type change shows it, is refused as one.
func TestDiscardOfTypeChangeToRepositoryRefused(t *testing.T) {
	r, dir := newRepo(t)
	os.Remove(filepath.Join(dir, "one.txt"))
	nested := filepath.Join(dir, "one.txt")
	git(t, dir, "init", "-q", nested)
	write(t, nested, "precious", "precious\n")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "-q", "-m", "its own history")
	f := fileNamed(t, load(t, r), "one.txt")
	if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard %q: %v", f.ShortStatus, err)
	}
	if got := readFile(t, nested, "precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// A conflicted file, replaced since by a directory of files, isn't
// brought back over them.
func TestDiscardOfConflictTurnedDirectoryRefused(t *testing.T) {
	r, dir, _ := conflictSince(t)
	os.Remove(filepath.Join(dir, "one.txt"))
	os.Mkdir(filepath.Join(dir, "one.txt"), 0o755)
	write(t, dir, "one.txt/precious", "precious\n")
	f := fileNamed(t, load(t, r), "one.txt")
	if err := r.Discard(f); !errors.Is(err, ErrStale) {
		t.Fatalf("discard %q: %v", f.ShortStatus, err)
	}
	if got := readFile(t, dir, "one.txt/precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// A file whose directory is a file since is stale, said so.
func TestDiscardUnderDirectoryTurnedFileIsStale(t *testing.T) {
	r, dir := newRepo(t)
	os.Mkdir(filepath.Join(dir, "d"), 0o755)
	write(t, dir, "d/f.txt", "f\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "d")
	os.RemoveAll(filepath.Join(dir, "d"))
	write(t, dir, "d", "precious\n")
	if err := r.Discard(fileNamed(t, load(t, r), "d/f.txt")); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "d"); got != "precious\n" {
		t.Fatalf("d: %q", got)
	}
}

// nestedAt makes a repository at path in dir, with a file in it, and
// gives its commit.
func nestedAt(t *testing.T, dir, path string) string {
	nested := filepath.Join(dir, path)
	git(t, dir, "init", "-q", nested)
	write(t, nested, "precious", "precious\n")
	git(t, nested, "add", ".")
	git(t, nested, "commit", "-q", "-m", "its own history")
	return git(t, nested, "rev-parse", "HEAD")
}

// A submodule's commit only in the index, or only below the path, isn't
// what checkout brings back, HEAD's file: the repository there is
// refused as one.
func TestDiscardOverRepositoryWhereHeadHasAFile(t *testing.T) {
	t.Run("staged type change", func(t *testing.T) {
		r, dir := newRepo(t)
		os.Remove(filepath.Join(dir, "one.txt"))
		nestedAt(t, dir, "one.txt")
		git(t, dir, "add", "one.txt")
		f := fileNamed(t, load(t, r), "one.txt")
		if err := r.Discard(f); !errors.Is(err, ErrNestedRepository) {
			t.Fatalf("discard %q: %v", f.ShortStatus, err)
		}
		if got := readFile(t, dir, "one.txt/precious"); got != "precious\n" {
			t.Fatalf("precious: %q", got)
		}
	})
	t.Run("submodule below", func(t *testing.T) {
		r, dir := newRepo(t)
		git(t, dir, "rm", "-q", "--cached", "one.txt")
		os.Remove(filepath.Join(dir, "one.txt"))
		os.Mkdir(filepath.Join(dir, "one.txt"), 0o755)
		nestedAt(t, dir, "one.txt/child")
		git(t, dir, "add", "one.txt/child")
		f := fileNamed(t, load(t, r), "one.txt")
		if err := r.Discard(f); err == nil {
			t.Fatalf("discarded %q over a repository", f.ShortStatus)
		}
		if got := readFile(t, dir, "one.txt/child/precious"); got != "precious\n" {
			t.Fatalf("precious: %q", got)
		}
	})
	t.Run("conflict", func(t *testing.T) {
		r, dir := newRepo(t)
		blob := git(t, dir, "rev-parse", "HEAD:one.txt")
		os.Remove(filepath.Join(dir, "one.txt"))
		h := nestedAt(t, dir, "one.txt")
		cmd := exec.Command("git", "update-index", "--index-info")
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader("0 0000000000000000000000000000000000000000\tone.txt\n" +
			"160000 " + h + " 1\tone.txt\n100644 " + blob + " 2\tone.txt\n100644 " + blob + " 3\tone.txt\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("update-index: %v\n%s", err, out)
		}
		f := fileNamed(t, load(t, r), "one.txt")
		if err := r.Discard(f); err == nil {
			t.Fatalf("discarded %q over a repository", f.ShortStatus)
		}
		if got := readFile(t, dir, "one.txt/precious"); got != "precious\n" {
			t.Fatalf("precious: %q", got)
		}
	})
}

// A file where HEAD has a submodule isn't the submodule's directory:
// checkout would take it away for an empty one.
func TestDiscardOfFileWhereHeadHasSubmodule(t *testing.T) {
	r, dir := newRepo(t)
	nestedAt(t, dir, "sub")
	git(t, dir, "add", "sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	os.RemoveAll(filepath.Join(dir, "sub"))
	write(t, dir, "sub", "precious\n")
	f := fileNamed(t, load(t, r), "sub")
	if err := r.Discard(f); !errors.Is(err, ErrStale) {
		t.Fatalf("discard %q: %v", f.ShortStatus, err)
	}
	if got := readFile(t, dir, "sub"); got != "precious\n" {
		t.Fatalf("sub: %q", got)
	}
}

// Discarding a submodule's change leaves what is in the submodule be,
// even with submodule.recurse, which would have checkout reset it.
func TestDiscardOfSubmoduleChangeKeepsItsWork(t *testing.T) {
	r, dir := newRepo(t)
	src := t.TempDir()
	git(t, src, "init", "-q", "-b", "main")
	write(t, src, "lib.txt", "v1\n")
	git(t, src, "add", ".")
	git(t, src, "commit", "-q", "-m", "v1")
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", src, "sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	sub := filepath.Join(dir, "sub")
	git(t, sub, "config", "user.name", "Test")
	git(t, sub, "config", "user.email", "test@example.com")
	write(t, sub, "lib.txt", "v2\n")
	git(t, sub, "commit", "-q", "-am", "v2")
	write(t, sub, "lib.txt", "work in progress\n")
	git(t, dir, "config", "submodule.recurse", "true")
	f := fileNamed(t, load(t, r), "sub")
	if err := r.Discard(f); err != nil {
		t.Fatalf("discard %q: %v", f.ShortStatus, err)
	}
	if got := readFile(t, sub, "lib.txt"); got != "work in progress\n" {
		t.Fatalf("lib.txt: %q", got)
	}
}

// An undo's hard reset leaves what is in a submodule be, even with
// submodule.recurse, which would have the reset reset it: its auto-stash
// saves only the superproject's changes.
func TestUndoHardResetKeepsSubmoduleWork(t *testing.T) {
	r, dir := newRepo(t)
	src := t.TempDir()
	git(t, src, "init", "-q", "-b", "main")
	write(t, src, "lib.txt", "v1\n")
	git(t, src, "add", ".")
	git(t, src, "commit", "-q", "-m", "v1")
	git(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "-q", src, "sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	if err := r.DropCommit(load(t, r).Commits, 1); err != nil { // "three"
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	write(t, sub, "lib.txt", "work in progress\n")
	git(t, dir, "config", "submodule.recurse", "true")
	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyUndo(s, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, sub, "lib.txt"); got != "work in progress\n" {
		t.Fatalf("lib.txt: %q", got)
	}
}

// A submodule's changes are discarded as before: its directory is its own.
func TestDiscardOfSubmoduleChange(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "sub")
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "first")
	git(t, dir, "add", "sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "second")
	f := fileNamed(t, load(t, r), "sub")
	if err := r.Discard(f); err != nil {
		t.Fatalf("discard %q: %v", f.ShortStatus, err)
	}
	if _, err := os.Stat(filepath.Join(nested, ".git")); err != nil {
		t.Fatalf("submodule gone: %v", err)
	}
}

// A conflict of a submodule's commits added by both, with a file where
// its directory would be, isn't discarded over the file.
func TestDiscardOfBothAddedRepositoryRefusesFileAtItsPath(t *testing.T) {
	r, dir := newRepo(t)
	h := git(t, dir, "rev-parse", "HEAD")
	cmd := exec.Command("git", "update-index", "--index-info")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("160000 " + h + " 2\tsub\n160000 " + h + " 3\tsub\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-index: %v\n%s", err, out)
	}
	write(t, dir, "sub", "precious\n")
	f := fileNamed(t, load(t, r), "sub")
	if f.ShortStatus != "AA" {
		t.Fatalf("status %q", f.ShortStatus)
	}
	if err := r.Discard(f); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "sub"); got != "precious\n" {
		t.Fatalf("sub: %q", got)
	}
}

// An empty directory where a file was shown, as a submodule's not checked
// out, holds nothing to lose: a conflict of it is discarded.
func TestDiscardOfConflictAtEmptyDirectory(t *testing.T) {
	r, dir := newRepo(t)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	h := git(t, dir, "rev-parse", "HEAD")
	cmd := exec.Command("git", "update-index", "--index-info")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("160000 " + h + " 1\tsub\n160000 " + h + " 3\tsub\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("update-index: %v\n%s", err, out)
	}
	f := fileNamed(t, load(t, r), "sub")
	if f.ShortStatus != "DU" {
		t.Fatalf("status %q", f.ShortStatus)
	}
	if err := r.Discard(f); err != nil {
		t.Fatalf("discard: %v", err)
	}
}

// A rename to a repository's path, refused, is refused whole: its old
// path isn't brought back first.
func TestDiscardOfRenamedRepositoryRefusedWhole(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "sub")
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	git(t, dir, "add", "sub")
	git(t, dir, "commit", "-q", "-m", "sub")
	git(t, dir, "mv", "sub", "sub2")
	s := load(t, r)
	if len(s.Files) != 1 || !s.Files[0].IsRename() {
		t.Fatalf("files %v", s.Files)
	}
	if err := r.Discard(s.Files[0]); !errors.Is(err, ErrNestedRepository) {
		t.Fatalf("discard: %v", err)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "R  sub -> sub2" {
		t.Fatalf("status: %q", st)
	}
}

// A repository shown, then replaced by a plain directory, is not deleted
// as the repository the user confirmed.
func TestDeleteNestedRepositoryRefusesWhatIsNoLongerOne(t *testing.T) {
	r, dir := newRepo(t)
	nested := filepath.Join(dir, "vendored")
	git(t, dir, "init", "-q", nested)
	git(t, nested, "commit", "-q", "--allow-empty", "-m", "its own history")
	f := fileNamed(t, load(t, r), "vendored")
	os.RemoveAll(nested)
	os.Mkdir(nested, 0o755)
	write(t, nested, "precious", "precious\n")
	if err := r.DeleteNestedRepository(f); !errors.Is(err, ErrStale) {
		t.Fatalf("delete: %v", err)
	}
	if got := readFile(t, nested, "precious"); got != "precious\n" {
		t.Fatalf("precious: %q", got)
	}
}

// The staging view's discard of a whole untracked file acts on it as shown:
// staged since, it isn't the untracked file the user chose to throw away.
func TestDiscardLinesOfFileStagedSinceRefused(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "new.txt", "new\n")
	f := fileNamed(t, load(t, r), "new.txt")
	diff, err := r.FileDiff(f, false)
	if err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "new.txt")
	if err := r.ApplyLines(f, diff, linesMatching(diff, "new"), DiscardLines); !errors.Is(err, ErrStale) {
		t.Fatalf("discard lines: %v", err)
	}
	if got := readFile(t, dir, "new.txt"); got != "new\n" {
		t.Fatalf("new.txt: %q", got)
	}
}

// A discard acts on the file as the user saw it: one staged since isn't
// the untracked file they chose to throw away.
func TestDiscardOfFileChangedSinceRefused(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "new.txt", "new\n")
	f := fileNamed(t, load(t, r), "new.txt")
	git(t, dir, "add", "new.txt")

	if err := r.Discard(f); !errors.Is(err, ErrStale) {
		t.Fatalf("discard: %v", err)
	}
	if got := readFile(t, dir, "new.txt"); got != "new\n" {
		t.Fatalf("new.txt: %q", got)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "A  new.txt" {
		t.Fatalf("status: %q", st)
	}
}

// conflictSince loads a snapshot with one.txt and three.txt changed, then,
// as another app would, without loading again, sets one.txt's change aside
// and starts a merge that conflicts on it.
func conflictSince(t *testing.T) (*Repo, string, *Snapshot) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "side")
	write(t, dir, "one.txt", "side\n")
	git(t, dir, "commit", "-q", "-am", "side")
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "one.txt", "main\n")
	git(t, dir, "commit", "-q", "-am", "main")
	write(t, dir, "one.txt", "changed\n")
	write(t, dir, "three.txt", "changed\n")
	s := load(t, r)
	git(t, dir, "checkout", "--", "one.txt")
	cmd := exec.Command("git", "merge", "side")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("merge didn't conflict")
	}
	return r, dir, s
}

func TestStageAllRefusesConflictSince(t *testing.T) {
	r, dir, s := conflictSince(t)
	if err := r.ToggleAllStaged(s.Files); !errors.Is(err, ErrStale) {
		t.Fatalf("stage all: %v", err)
	}
	if st := git(t, dir, "status", "--porcelain"); !strings.Contains(st, "UU one.txt") || strings.Contains(st, "M  three.txt") {
		t.Fatalf("status: %q", st)
	}
}

// A stash is acted on by its place in the stash, which another stash made
// since moves: the one acted on must still be the one shown.
func TestStashActionsRefuseShiftedEntry(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "one.txt", "old\n")
	git(t, dir, "stash", "push", "-q", "-m", "old")
	old := load(t, r).Stash[0]
	write(t, dir, "two.txt", "new\n")
	git(t, dir, "stash", "push", "-q", "-m", "new")

	for name, act := range map[string]func(*Repo) error{
		"drop":  func(r *Repo) error { return r.StashDrop(old) },
		"pop":   func(r *Repo) error { return r.StashPop(old) },
		"apply": func(r *Repo) error { return r.StashApply(old) },
	} {
		if err := act(r); !errors.Is(err, ErrStale) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if got := git(t, dir, "stash", "list", "--format=%s"); got != "On main: new\nOn main: old" {
		t.Fatalf("stash: %q", got)
	}
	if st := git(t, dir, "status", "--porcelain"); st != "" {
		t.Fatalf("status: %q", st)
	}
}

// git stash passes over a file marked assume-unchanged, and the hard
// reset of an undo then overwrites its edits, unsaved. (A skip-worktree
// file, which sparse checkouts mark, git's reset leaves alone.)
func TestUndoHardResetRefusesAssumeUnchangedEdits(t *testing.T) {
	r, dir := newRepo(t)
	if err := r.DropCommit(load(t, r).Commits, 1); err != nil { // "two"
		t.Fatal(err)
	}
	git(t, dir, "update-index", "--assume-unchanged", "one.txt")
	write(t, dir, "one.txt", "hidden edit\n")

	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyUndo(s, false); err == nil {
		t.Fatal("undid over a hidden edit")
	}
	if got := readFile(t, dir, "one.txt"); got != "hidden edit\n" {
		t.Fatalf("one.txt: %q", got)
	}
}

// Files marked assume-unchanged with nothing a reset would lose don't stop
// an undo: one unchanged, a symlink unchanged (whose blob is where it
// leads, not what is there), and one deleted, which the reset brings back.
func TestUndoHardResetWithAssumeUnchangedButNothingToLose(t *testing.T) {
	r, dir := newRepo(t)
	os.Symlink("one.txt", filepath.Join(dir, "link"))
	git(t, dir, "add", "link")
	git(t, dir, "commit", "-q", "-m", "link")
	if err := r.DropCommit(load(t, r).Commits, 2); err != nil { // "two"
		t.Fatal(err)
	}
	git(t, dir, "update-index", "--assume-unchanged", "one.txt", "link", "three.txt")
	os.Remove(filepath.Join(dir, "three.txt"))

	s, err := r.FindUndo(false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.ApplyUndo(s, false); err != nil {
		t.Fatal(err)
	}
	if got := subjects(t, dir); !slices.Equal(got, []string{"link", "three", "two", "one"}) {
		t.Fatalf("log after undo: %v", got)
	}
}

// A file marked assume-unchanged is hashed by its own name, however it is
// spelled: a quote at its start or a CR at its end isn't another file's.
func TestUndoHardResetRefusesAssumeUnchangedEditsOfOddNames(t *testing.T) {
	for _, name := range []string{`"decoy.txt"`, "decoy.txt\r"} {
		t.Run(strings.TrimSpace(name), func(t *testing.T) {
			r, dir := newRepo(t)
			write(t, dir, "decoy.txt", "same\n")
			write(t, dir, name, "same\n")
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-q", "-m", "decoys")
			if err := r.DropCommit(load(t, r).Commits, 2); err != nil { // "two"
				t.Fatal(err)
			}
			git(t, dir, "update-index", "--assume-unchanged", "--", name)
			write(t, dir, name, "hidden edit\n")

			s, err := r.FindUndo(false)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.ApplyUndo(s, false); err == nil {
				t.Fatal("undid over a hidden edit")
			}
			if got := readFile(t, dir, name); got != "hidden edit\n" {
				t.Fatalf("%q: %q", name, got)
			}
		})
	}
}

// bareRemote adds a bare repository as a remote of dir, named name.
func bareRemote(t *testing.T, dir, name string) string {
	bare := t.TempDir()
	git(t, bare, "init", "-q", "--bare")
	git(t, dir, "remote", "add", "--", name, bare)
	return bare
}

// git push takes a remote named "--force", or "--receive-pack=…", for its
// option: no push may even run with one.
func TestPushRefusesOptionLikeRemote(t *testing.T) {
	r, dir := newRepo(t)
	bareRemote(t, dir, "--force")
	var pushes []string
	logged, err := Open(dir, Options{ConfigDir: filepath.Join(t.TempDir(), "config"), OnCommand: func(cmd string, _ bool) {
		if strings.Contains(cmd, "push") {
			pushes = append(pushes, cmd)
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logged.Close)
	if err := logged.Push(load(t, r).Head, "--force", false); err == nil {
		t.Fatal("pushed to a remote named --force")
	}
	if len(pushes) != 0 {
		t.Fatalf("ran %q", pushes)
	}
}

// A branch's first push goes where git's config says, or to the only
// remote; with several and no word from the config, the user is asked.
func TestPushRemoteOfNewBranch(t *testing.T) {
	r, dir := newRepo(t)
	bareRemote(t, dir, "origin")
	head := load(t, r).Head
	pushRemote := func(want string, wantAsk bool) {
		t.Helper()
		if remote, ask, err := r.PushRemote(head); remote != want || ask != wantAsk || err != nil {
			t.Fatalf("remote %q, ask %v, %v; want %q, ask %v", remote, ask, err, want, wantAsk)
		}
	}
	pushRemote("origin", false)
	public := bareRemote(t, dir, "public")
	pushRemote("origin", true)
	git(t, dir, "config", "remote.pushDefault", "public")
	pushRemote("public", false)
	git(t, dir, "config", "branch.main.pushRemote", "origin")
	pushRemote("origin", false)

	if err := r.Push(head, "public", false); err != nil {
		t.Fatal(err)
	}
	if got := git(t, public, "rev-parse", "main"); got != git(t, dir, "rev-parse", "main") {
		t.Fatalf("public has main at %q", got)
	}
	if up := git(t, dir, "rev-parse", "--abbrev-ref", "main@{upstream}"); up != "public/main" {
		t.Fatalf("upstream %q", up)
	}
	// A path or URL isn't one of the remotes, though git would push to it.
	elsewhere := t.TempDir()
	git(t, elsewhere, "init", "-q", "--bare")
	if err := r.Push(head, elsewhere, false); err == nil {
		t.Fatal("pushed to a path that isn't a remote")
	}
	if refs := git(t, elsewhere, "for-each-ref"); refs != "" {
		t.Fatalf("pushed to %s: %q", elsewhere, refs)
	}
}

// A remote's name is all of it: "private public" doesn't make "public" a
// remote, which a push would take for a path.
func TestPushRemoteNamesAreWhole(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "config", "remote.private public.url", t.TempDir())
	git(t, dir, "init", "-q", "--bare", "public")
	git(t, dir, "config", "branch.main.pushRemote", "public")
	head := load(t, r).Head
	if remote, _, err := r.PushRemote(head); err == nil {
		t.Fatalf("pushRemote names no remote, yet %q", remote)
	}
	if err := r.Push(head, "public", false); err == nil {
		t.Fatal("pushed to a path that isn't a remote")
	}
	if refs := git(t, filepath.Join(dir, "public"), "for-each-ref"); refs != "" {
		t.Fatalf("pushed to public: %q", refs)
	}
}

// A push destination git's config names, but that isn't a remote (a typo,
// or a remote since removed), fails: it isn't for the app to go elsewhere.
func TestPushRemoteConfiguredButMissing(t *testing.T) {
	r, dir := newRepo(t)
	bareRemote(t, dir, "public")
	head := load(t, r).Head
	for _, key := range []string{"remote.pushDefault", "branch.main.pushRemote"} {
		git(t, dir, "config", key, "private")
		if remote, _, err := r.PushRemote(head); err == nil {
			t.Fatalf("%s names no remote, yet %q", key, remote)
		}
	}
}

// A diff past maxDiffSize isn't read whole into memory, nor shown.
func TestDiffTooLarge(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "big.txt", strings.Repeat("a line of a large file\n", maxDiffSize/20))
	if _, err := r.FileDiff(fileNamed(t, load(t, r), "big.txt"), false); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("work tree diff: %v", err)
	}
	git(t, dir, "add", "big.txt")
	git(t, dir, "commit", "-q", "-m", "big")
	s := load(t, r)
	files, err := r.CommitFiles(s.Commits[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitFileDiff(s.Commits[0], files[0]); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("commit diff: %v", err)
	}
	if d, err := r.CommitFileDiff(s.Commits[1], &models.CommitFile{Path: "three.txt"}); err != nil || !strings.Contains(d, "+three") {
		t.Fatalf("small diff %q, %v", d, err)
	}
}

// textconv commits a.bin, then has git diff it through script, as a
// repository's config can.
func textconv(t *testing.T, script string) (*Repo, *models.Commit, *models.CommitFile) {
	r, dir := newRepo(t)
	conv := filepath.Join(t.TempDir(), "conv.sh")
	if err := os.WriteFile(conv, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, ".gitattributes", "*.bin diff=conv\n")
	write(t, dir, "a.bin", "a\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "bin")
	git(t, dir, "config", "diff.conv.textconv", conv)
	c := load(t, r).Commits[0]
	return r, c, &models.CommitFile{Path: "a.bin"}
}

// What a diff's command says on its standard error is bounded as its
// output is.
func TestDiffErrorOutputBounded(t *testing.T) {
	r, c, f := textconv(t, "head -c 20000000 /dev/zero >&2\nexit 1\n")
	_, err := r.CommitFileDiff(c, f)
	if err == nil {
		t.Fatal("no error")
	}
	if len(err.Error()) > 1<<20 {
		t.Fatalf("error of %d bytes", len(err.Error()))
	}
}

// A diff too large returns at once, though a process git started holds
// on to its output: the repository is everyone's to wait on meanwhile.
func TestDiffTooLargeDoesntWaitOnWhatHoldsItsOutput(t *testing.T) {
	r, c, f := textconv(t, "sleep 20 >/dev/null &\nyes 'a line of a large file' | head -c 20000000\n")
	start := time.Now()
	if _, err := r.CommitFileDiff(c, f); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("diff: %v", err)
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("took %v", d)
	}
}

// A stash made by another app while a pop runs, after its check, just
// before it applies, doesn't make the pop take that stash, nor drop it.
func TestStashPopDuringStashMadeElsewhere(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "one.txt", "old\n")
	git(t, dir, "stash", "push", "-q", "-m", "old")
	made := false
	logged, err := Open(dir, Options{ConfigDir: filepath.Join(t.TempDir(), "config"), OnCommand: func(cmd string, _ bool) {
		if strings.HasPrefix(cmd, "git stash") && !made {
			made = true
			write(t, dir, "two.txt", "new\n")
			git(t, dir, "stash", "push", "-q", "-m", "new")
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(logged.Close)
	if err := logged.StashPop(load(t, r).Stash[0]); !errors.Is(err, ErrStale) {
		t.Fatalf("pop: %v", err)
	}
	if got := git(t, dir, "stash", "list", "--format=%s"); got != "On main: new\nOn main: old" {
		t.Fatalf("stash: %q", got)
	}
	if got := readFile(t, dir, "one.txt"); got != "old\n" {
		t.Fatalf("one.txt: %q", got)
	}
}

// A stash made by another app just before a drop's command, after its
// check, moves the entry the drop's index names: the entry dropped in its
// place is stored back, and none is lost.
// git's words, which say which entry it dropped, are read in English
// whatever the user's language.
func TestStashDropOfStashMadeJustBefore(t *testing.T) {
	for _, lang := range []string{"C", "de_DE.UTF-8", "zh_CN.UTF-8"} {
		t.Run(lang, func(t *testing.T) {
			t.Setenv("LC_ALL", lang)
			testStashDropOfStashMadeJustBefore(t)
		})
	}
}

func testStashDropOfStashMadeJustBefore(t *testing.T) {
	for name, act := range map[string]func(*Repo, *models.StashEntry) error{
		"drop": (*Repo).StashDrop,
		"pop":  (*Repo).StashPop,
	} {
		t.Run(name, func(t *testing.T) {
			r, dir := newRepo(t)
			write(t, dir, "one.txt", "old\n")
			git(t, dir, "stash", "push", "-q", "-m", "old")
			made := false
			logged, err := Open(dir, Options{ConfigDir: filepath.Join(t.TempDir(), "config"), OnCommand: func(cmd string, _ bool) {
				if strings.HasPrefix(cmd, "git stash drop") && !made {
					made = true
					write(t, dir, "two.txt", "new\n")
					git(t, dir, "stash", "push", "-q", "-m", "new")
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(logged.Close)
			if err := act(logged, load(t, r).Stash[0]); !errors.Is(err, ErrStale) {
				t.Fatalf("%s: %v", name, err)
			}
			if got := git(t, dir, "stash", "list", "--format=%s"); got != "On main: new\nOn main: old" {
				t.Fatalf("stash: %q", got)
			}
		})
	}
}

// Staging a file that has merge conflicts since it was shown would stage
// its markers.
func TestStageRefusesConflictSince(t *testing.T) {
	r, dir, s := conflictSince(t)
	if err := r.ToggleStaged(fileNamed(t, s, "one.txt")); !errors.Is(err, ErrStale) {
		t.Fatalf("toggle: %v", err)
	}
	if st := git(t, dir, "status", "--porcelain"); !strings.Contains(st, "UU one.txt") {
		t.Fatalf("status: %q", st)
	}
}

// A file turned since into a directory with conflicts in it would have
// them staged, markers and all, as git add stages a directory's files.
func TestStageRefusesConflictsUnderFileTurnedDirectory(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "folder", "a file\n")
	git(t, dir, "add", "folder")
	git(t, dir, "commit", "-q", "-m", "folder")
	for _, branch := range []string{"side", "other"} {
		git(t, dir, "checkout", "-q", "-b", branch, "main")
		git(t, dir, "rm", "-q", "folder")
		os.Mkdir(filepath.Join(dir, "folder"), 0o755)
		write(t, dir, "folder/child.txt", branch+"\n")
		git(t, dir, "add", "folder")
		git(t, dir, "commit", "-q", "-m", branch)
	}
	git(t, dir, "checkout", "-q", "main")
	write(t, dir, "folder", "changed\n")
	f := fileNamed(t, load(t, r), "folder")
	git(t, dir, "checkout", "--", "folder")
	git(t, dir, "checkout", "-q", "other")
	cmd := exec.Command("git", "merge", "side")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("merge didn't conflict")
	}
	if err := r.ToggleStaged(f); !errors.Is(err, ErrStale) {
		t.Fatalf("toggle: %v", err)
	}
	if st := git(t, dir, "status", "--porcelain"); !strings.Contains(st, "AA folder/child.txt") && !strings.Contains(st, "UU folder/child.txt") {
		t.Fatalf("status: %q", st)
	}
}

// Staging a file staged already since it was shown, as by a key pressed
// twice, is no harm: it stays staged, with no error.
func TestStageOfFileStagedSince(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "one.txt", "changed\n")
	f := fileNamed(t, load(t, r), "one.txt")
	for range 2 {
		if err := r.ToggleStaged(f); err != nil {
			t.Fatal(err)
		}
	}
	if st := git(t, dir, "status", "--porcelain"); st != "M  one.txt" {
		t.Fatalf("status: %q", st)
	}
}
