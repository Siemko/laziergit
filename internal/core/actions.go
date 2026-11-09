package core

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/stefanhaller/git-todo-parser/todo"
)

// Files

func (r *Repo) ToggleStaged(f *models.File) error {
	if f.HasUnstagedChanges && f.HasInlineMergeConflicts {
		return errConflicted(f.Path)
	}
	return r.do(func(g *commands.GitCommand) error {
		if f.HasUnstagedChanges {
			// A conflict since the file was shown would be staged, markers
			// and all, its own or, become a directory, its files'; any
			// other change since is staged as it would be.
			for _, now := range r.git.Loaders.FileLoader.GetStatusFiles(git_commands.GetStatusFileOptions{}) {
				if (now.Path == f.Path || strings.HasPrefix(now.Path, f.Path+"/")) && now.HasInlineMergeConflicts {
					return staleError(now.Path + " has merge conflicts since " + f.Path + " was shown: look at it again")
				}
			}
			return g.WorkingTree.StageFile(literal(f.Path))
		}
		return g.WorkingTree.UnStageFile(literals(f.Names()), f.Tracked)
	})
}

// ToggleAllStaged stages every change, or unstages them all when all are
// staged already.
func (r *Repo) ToggleAllStaged(files []*models.File) error {
	allStaged := true
	for _, f := range files {
		if f.HasUnstagedChanges {
			allStaged = false
		}
	}
	if !allStaged {
		for _, f := range files {
			if f.HasInlineMergeConflicts {
				return errConflicted(f.Path)
			}
		}
	}
	return r.do(func(g *commands.GitCommand) error {
		if allStaged {
			return g.WorkingTree.UnstageAll()
		}
		// A conflict since the files were shown would be staged, markers
		// and all: the check above was of the files as shown.
		for _, now := range r.git.Loaders.FileLoader.GetStatusFiles(git_commands.GetStatusFileOptions{}) {
			if now.HasInlineMergeConflicts {
				return staleError(now.Path + " has merge conflicts since the changes were shown: look at them again")
			}
		}
		return g.WorkingTree.StageAll(false)
	})
}

// ErrNestedRepository is the error of discarding a repository inside the
// work tree, which DeleteNestedRepository does, with all its history.
var ErrNestedRepository = errors.New("it is a repository of its own, with its history")

func (r *Repo) Discard(f *models.File) error {
	if f.IsNestedRepository() {
		return ErrNestedRepository
	}
	return r.do(func(g *commands.GitCommand) error {
		if err := r.checkStatus(f); err != nil {
			return err
		}
		return r.discardFile(g, f, false)
	})
}

// checkStatus fails with ErrStale when the file's status is no longer the
// one f was loaded with, as after a git command in another app: an action
// chosen for a file as shown must not act on it as it is now.
func (r *Repo) checkStatus(f *models.File) error {
	if now := r.statusOf(f.Path); now != nil && now.ShortStatus == f.ShortStatus && now.PreviousPath == f.PreviousPath {
		return nil
	}
	return staleError(f.Path + " changed since it was shown: look at it again")
}

// statusOf is the file at path as git status has it now, nil for one with
// no changes; the repository must be current.
func (r *Repo) statusOf(path string) *models.File {
	for _, now := range r.git.Loaders.FileLoader.GetStatusFiles(git_commands.GetStatusFileOptions{}) {
		if now.Path == path {
			return now
		}
	}
	return nil
}

// discardFile throws a file's changes away, from the index and the work
// tree, as lazygit's DiscardAllFileChanges does, with its paths literal
// pathspecs for git and checked paths for the file system (see
// removeUntracked, which deleteRepos is for).
func (r *Repo) discardFile(g *commands.GitCommand, f *models.File, deleteRepos bool) error {
	if f.IsRename() {
		before, after, err := g.WorkingTree.BeforeAndAfterFileForRename(f)
		if err != nil {
			return err
		}
		// A file made at the old path since is what lazygit finds there
		// (untracked, as it is), not the old file: discarding it would
		// throw it away.
		if before.Added {
			return staleError(f.PreviousPath + " is a new file, not the one renamed: look at it again")
		}
		// The new path, refused, is refused before the old one is brought
		// back: the rename stays whole.
		if err := r.checkBeforeDiscard(after, deleteRepos); err != nil {
			return err
		}
		if err := r.discardFile(g, before, deleteRepos); err != nil {
			return err
		}
		return r.discardFile(g, after, deleteRepos)
	}
	if err := r.checkBeforeDiscard(f, deleteRepos); err != nil {
		return err
	}
	path := literal(f.Path)
	switch f.ShortStatus {
	case "AA":
		if err := r.checkoutWithoutRecursing("--ours", "--", path); err != nil {
			return err
		}
		return r.gitRun("add", "--", path)
	case "DU":
		return r.gitRun("rm", "--", path)
	}
	if f.HasStagedChanges || f.HasMergeConflicts {
		if err := r.gitRun("reset", "--", path); err != nil {
			return err
		}
	}
	if f.ShortStatus == "DD" || f.ShortStatus == "AU" {
		return nil
	}
	if f.Added {
		// Not in HEAD: discarding it is removing it.
		return r.removeUntracked(f, deleteRepos)
	}
	return r.checkoutWithoutRecursing("--", path)
}

// checkBeforeDiscard looks, on disk, at what discarding f would remove or
// replace, before anything is done (as well as, for a file not in HEAD,
// just before it is removed): refused, it is left as it was, in the index
// too. That is a file not in HEAD, one git rm takes (DU), one added by
// both (AA), whose ours checked out replaces what is there, unless ours is
// a submodule's commit, which leaves its directory be; and, for all else
// but what acts on the index only (DD, AU), what git checkout, bringing
// the file back, takes away from its path (see checkRestorable).
func (r *Repo) checkBeforeDiscard(f *models.File, deleteRepos bool) error {
	switch {
	case f.ShortStatus == "AA" && r.oursIsSubmodule(f.Path):
		return r.withRoot(f, func(root *os.Root) error {
			st, err := lstat(root, f.Path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return nil
			case err != nil:
				return err
			case !st.IsDir():
				return staleError(f.Path + " is a file, where the submodule's directory would be: look at it again")
			}
			return nil
		})
	case f.ShortStatus == "AA", f.ShortStatus == "DU", f.Added:
		return r.withRoot(f, func(root *os.Root) error { return checkRemovable(root, f, deleteRepos) })
	case f.ShortStatus == "DD", f.ShortStatus == "AU":
		return nil
	}
	return r.withRoot(f, func(root *os.Root) error { return r.checkRestorable(root, f) })
}

// checkRestorable looks at the path of f, a file git checkout is to bring
// back, which takes away whatever is there: a directory, as one of new
// files, or a repository, as a type change shows one, and, for a file
// deleted, anything, listed or not (ignored, or a directory's files,
// listed each for itself). Nothing there, an empty directory, or a
// submodule's own is no matter.
func (r *Repo) checkRestorable(root *os.Root, f *models.File) error {
	st, err := lstat(root, f.Path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return err
	case deleted(f) && !st.IsDir():
		return staleError(f.Path + " has something new at its path since it was deleted: look at it again")
	case st.IsDir() && emptyDir(root, f.Path):
		return nil
	}
	sub, err := r.headIsSubmodule(f.Path)
	switch {
	case err != nil:
		return err
	case !st.IsDir() && sub:
		return staleError(f.Path + " is a file, where the submodule's directory would be: look at it again")
	case !st.IsDir() || sub:
		return nil
	}
	held, err := holdsRepository(root, filepath.ToSlash(f.Path))
	switch {
	case err != nil:
		return err
	case held:
		return ErrNestedRepository
	}
	return staleError(f.Path + " is a directory now, not the file shown: look at it again")
}

// lstat is root.Lstat, a directory on the way that is a file now stale.
func lstat(root *os.Root, path string) (fs.FileInfo, error) {
	st, err := root.Lstat(path)
	if errors.Is(err, syscall.ENOTDIR) {
		return nil, staleError(path + " is in a directory that is a file now: look at it again")
	}
	return st, err
}

// headIsSubmodule reports whether HEAD has path itself as a submodule's
// commit: what a discard's checkout brings back, after its reset puts
// HEAD's entry in the index, or as the index has it, the same, with no
// change staged. A submodule's commit only in the index, or in a
// conflict's stage, is the change thrown away. HEAD unreadable fails: a
// guess either way could let a checkout take what is there.
func (r *Repo) headIsSubmodule(path string) (bool, error) {
	out, err := r.git_("ls-tree", "-z", "HEAD", "--", literal(path))
	if err != nil {
		return false, fmt.Errorf("reading HEAD's %s: %w", path, err)
	}
	for _, e := range strings.Split(out, "\x00") {
		// "160000 commit <hash>\t<path>"
		meta, p, _ := strings.Cut(e, "\t")
		if p == path && strings.HasPrefix(meta, "160000 ") {
			return true, nil
		}
	}
	return false, nil
}

// checkoutWithoutRecursing is git checkout as a discard runs it: a
// submodule's directory left as it is, whatever submodule.recurse says,
// which would reset what is in it.
func (r *Repo) checkoutWithoutRecursing(args ...string) error {
	return r.gitRun(append([]string{"-c", "submodule.recurse=false", "checkout"}, args...)...)
}

// deleted reports whether f's status has it gone from the work tree,
// deleted there or from the index, so that whatever is at its path now is
// new. Deleted by them (UD), ours stays, and both (DD) brings nothing back.
func deleted(f *models.File) bool {
	s := f.ShortStatus
	return s == "D " || len(s) == 2 && s[1] == 'D' && s[0] != 'U' && s[0] != 'D'
}

// oursIsSubmodule reports whether the index's ours (stage 2) of a
// conflicted path is a submodule's commit.
func (r *Repo) oursIsSubmodule(path string) bool {
	out, _ := r.git_("ls-files", "-s", "-z", "--", literal(path))
	for _, e := range strings.Split(out, "\x00") {
		// "160000 <hash> 2\t<path>", of path itself, not one below it
		meta, p, _ := strings.Cut(e, "\t")
		if fields := strings.Fields(meta); p == path && len(fields) == 3 && fields[2] == "2" {
			return fields[0] == "160000"
		}
	}
	return false
}

// removeUntracked removes a file not in HEAD from the work tree, through
// the work tree's root, which no symlink leads out of, even one made
// since; a symlink goes itself, not what it leads to. What it removes is
// looked at on disk just before (checkRemovable).
func (r *Repo) removeUntracked(f *models.File, deleteRepos bool) error {
	return r.withRoot(f, func(root *os.Root) error {
		if err := checkRemovable(root, f, deleteRepos); err != nil {
			return err
		}
		return root.RemoveAll(f.Path)
	})
}

// withRoot runs fn with the work tree's root, f's path checked to be in it.
func (r *Repo) withRoot(f *models.File, fn func(root *os.Root) error) error {
	if !filepath.IsLocal(f.Path) {
		return fmt.Errorf("%s isn't a path in the work tree", f.Path)
	}
	root, err := os.OpenRoot(r.Path)
	if err != nil {
		return err
	}
	defer root.Close()
	return fn(root)
}

// checkRemovable looks at f's path on disk, as it is now, for removing it:
// a directory with a repository in it fails with ErrNestedRepository,
// unless deleteRepos, which, the other way, fails for a path that no
// longer has one; a directory where a file was shown, or a file where a
// directory was, is stale. A path gone is no matter.
func checkRemovable(root *os.Root, f *models.File, deleteRepos bool) error {
	st, err := lstat(root, f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	shownDir := f.DirectoryKind != models.NotADirectory
	if !st.IsDir() {
		switch {
		case deleteRepos:
			return staleError(f.Path + " is no longer a repository: look at it again")
		case shownDir:
			return staleError(f.Path + " is a file now, not the directory shown: look at it again")
		}
		return nil
	}
	held, err := holdsRepository(root, filepath.ToSlash(f.Path))
	switch {
	case err != nil:
		return err
	case held && !deleteRepos:
		return ErrNestedRepository
	case !held && deleteRepos:
		return staleError(f.Path + " is no longer a repository: look at it again")
	case !shownDir && !emptyDir(root, f.Path):
		// An empty one, as a submodule's not checked out, holds nothing
		// to lose.
		return staleError(f.Path + " is a directory now, not the file shown: look at it again")
	}
	return nil
}

// emptyDir reports whether dir, a directory of root, has nothing in it.
func emptyDir(root *os.Root, dir string) bool {
	d, err := root.Open(dir)
	if err != nil {
		return false
	}
	defer d.Close()
	entries, err := d.ReadDir(1)
	return len(entries) == 0 && errors.Is(err, io.EOF)
}

// holdsRepository reports whether dir, a directory of root, is or holds a
// repository anywhere in it: a directory with a .git, directory or file,
// as the disk finds it, by its own rules of spelling, as git does (".GIT"
// on a disk that doesn't tell case apart).
func holdsRepository(root *os.Root, dir string) (bool, error) {
	held := false
	err := fs.WalkDir(root.FS(), dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if _, err := root.Lstat(path.Join(p, ".git")); err == nil {
			held = true
			return fs.SkipAll
		}
		return nil
	})
	return held, err
}

// DeleteNestedRepository deletes a repository inside the work tree that
// isn't a submodule, with all its history.
func (r *Repo) DeleteNestedRepository(f *models.File) error {
	if !f.IsNestedRepository() {
		return errors.New(f.Path + " is not a repository")
	}
	return r.do(func(g *commands.GitCommand) error { return r.discardFile(g, f, true) })
}

// conflictError is a command stopping with an operation in progress, as
// a merge on conflicts: known from the repository's state rather than
// git's words, which follow the user's language.
type conflictError struct{ err error }

func (e conflictError) Error() string { return e.err.Error() }
func (e conflictError) Unwrap() error { return e.err }

// stopPoint is where a repository stands, to tell whether a command
// stopped it: the operation in progress, and the files in conflict with
// their versions (git ls-files -u).
type stopPoint struct {
	state    models.WorkingTreeState
	unmerged string
}

func (r *Repo) stopPoint(g *commands.GitCommand) stopPoint {
	unmerged, _ := r.git_("ls-files", "-u", "-z")
	return stopPoint{state: g.Status.WorkingTreeState(), unmerged: unmerged}
}

// doMayConflict is do for commands that stop on conflicts.
func (r *Repo) doMayConflict(fn func(g *commands.GitCommand) error) error {
	return r.do(func(g *commands.GitCommand) error {
		before := r.stopPoint(g)
		err := fn(g)
		if err == nil {
			return nil
		}
		// Stopped by this command, rather than refused while an operation
		// was in progress already: the operation started, or other files,
		// or other versions of them, came to conflict, as a continue or a
		// skip stopping on the next commit does. A commit that failed for
		// another reason, as a hook, leaves no conflict.
		after := r.stopPoint(g)
		if after.state.Any() && (after.state != before.state || after.unmerged != "" && after.unmerged != before.unmerged) {
			return conflictError{err}
		}
		return err
	})
}

// Commits

// Commit commits what is staged. Amending adds it to the last commit,
// whose message becomes summary and description when summary is given.
func (r *Repo) Commit(summary, description string, amend bool) error {
	if summary == "" && !amend {
		return errors.New("the commit needs a summary")
	}
	return r.do(func(g *commands.GitCommand) error {
		if !amend {
			return g.Commit.CommitCmdObj(summary, description, false).Run()
		}
		if err := g.Commit.AmendHead(); err != nil {
			return err
		}
		if summary == "" {
			return nil
		}
		return g.Commit.RewordLastCommit(summary, description).Run()
	})
}

// The commit actions below rewrite history with an interactive rebase,
// whose todo list lazygit's daemon (see HandleDaemon) edits.

func (r *Repo) DropCommit(commits []*models.Commit, i int) error {
	return r.doMayConflict(func(g *commands.GitCommand) error {
		if commits[i].IsMerge() {
			// A merge's todo is a merge line, not a pick to drop.
			return g.Rebase.DropMergeCommit(commits, i)
		}
		return g.Rebase.InteractiveRebase(commits, i, i, todo.Drop, "")
	})
}

// FixupCommit squashes a commit into the one below it, keeping that one's
// message.
func (r *Repo) FixupCommit(commits []*models.Commit, i int) error {
	if i+1 >= len(commits) {
		return errors.New("there is no commit below to fix up into")
	}
	if commits[i].IsMerge() {
		return errors.New("a merge commit can't be fixed up into another")
	}
	return r.doMayConflict(func(g *commands.GitCommand) error {
		return g.Rebase.InteractiveRebase(commits, i, i, todo.Fixup, "")
	})
}

func (r *Repo) MoveCommit(commits []*models.Commit, i, delta int) error {
	return r.doMayConflict(func(g *commands.GitCommand) error {
		if delta < 0 {
			return g.Rebase.MoveCommitsUp(commits, i, i)
		}
		return g.Rebase.MoveCommitsDown(commits, i, i)
	})
}

func (r *Repo) RewordCommit(commits []*models.Commit, i int, summary, description string) error {
	return r.do(func(g *commands.GitCommand) error {
		return g.Rebase.RewordCommit(commits, i, summary, description)
	})
}

func (r *Repo) CommitMessage(c *models.Commit) (string, error) {
	return with(r, func() (string, error) { return r.git.Commit.GetCommitMessage(c.Hash()) })
}

// Branches

// checkRef rejects a branch name git would read as an option: refs named
// so can't be made with git branch, but can be with git update-ref.
func checkRef(name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q isn't a branch name laziergit can use", name)
	}
	return nil
}

func (r *Repo) Checkout(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return r.checkout(name, nil) })
}

// checkout checks out a branch or commit, never paths: lazygit's checkout
// has no "--", so git would take a branch deleted since for the directory
// of its name, throwing that directory's changes away.
func (r *Repo) checkout(name string, env []string) error {
	return r.osCmd.Cmd.New([]string{"git", "checkout", name, "--"}).
		AddEnvVars("GIT_TERMINAL_PROMPT=0").AddEnvVars(env...).Run()
}

func (r *Repo) NewBranch(name, base string) error {
	if err := errors.Join(checkRef(name), checkRef(base)); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return g.Branch.New(name, base) })
}

// ErrNotMerged is the error of deleting, without forcing, a branch whose
// commits no other branch has.
var ErrNotMerged = errors.New("the branch isn't fully merged")

// branchExists reports whether a local branch is there; the repository must
// be current.
func (r *Repo) branchExists(name string) bool {
	_, err := r.git_("rev-parse", "-q", "--verify", "refs/heads/"+name)
	return err == nil
}

// branchMerged reports whether a branch is merged into HEAD and, when it
// has an upstream, into that, as git branch -d asks.
func (r *Repo) branchMerged(g *commands.GitCommand, name string) bool {
	if !g.Branch.IsAncestor(name, "HEAD") {
		return false
	}
	if _, err := r.git_("rev-parse", "-q", "--verify", name+"@{upstream}"); err != nil {
		return true // no upstream
	}
	return g.Branch.IsAncestor(name, name+"@{upstream}")
}

// DeleteBranch deletes a branch: unless force, only one merged, failing
// with ErrNotMerged otherwise.
func (r *Repo) DeleteBranch(name string, force bool) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		err := g.Branch.LocalDelete([]string{name}, force)
		// Whether it was refused for not being merged is known from the
		// branch, not git's words, which follow the user's language.
		// git refuses a branch not merged into HEAD, or into its upstream
		// when it has one; another refusal, as of a branch checked out in
		// another worktree, is git's own error.
		if err != nil && !force && r.branchExists(name) && !r.branchMerged(g, name) {
			return fmt.Errorf("%w: %w", ErrNotMerged, err)
		}
		return err
	})
}

func (r *Repo) Merge(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.doMayConflict(func(g *commands.GitCommand) error {
		return g.Branch.Merge(name, git_commands.MERGE_VARIANT_REGULAR)
	})
}

func (r *Repo) RebaseOnto(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.doMayConflict(func(g *commands.GitCommand) error { return g.Rebase.RebaseBranch(name) })
}

// Remotes

func (r *Repo) Fetch() error {
	return r.do(func(g *commands.GitCommand) error { return g.Sync.Fetch(gocui.NewFakeTask()) })
}

func (r *Repo) Pull() error {
	return r.doMayConflict(func(g *commands.GitCommand) error {
		return g.Sync.Pull(gocui.NewFakeTask(), git_commands.PullOptions{})
	})
}

// PushRemote is the remote the first push of head, a branch with no
// upstream yet, goes to: its branch.<name>.pushRemote, the repository's
// remote.pushDefault, or its only remote. With several remotes and neither
// set, ask is true: the user chooses, remote (origin, or the first) being
// the one to offer. A setting that names no remote fails: git would push
// to it as a path or a URL, and going to another remote in its place
// isn't for the app to decide.
func (r *Repo) PushRemote(head *models.Branch) (remote string, ask bool, err error) {
	if head == nil {
		return "", false, errors.New("check out a branch to push it")
	}
	type answer struct {
		remote string
		ask    bool
	}
	a, err := with(r, func() (answer, error) {
		remotes := r.remoteNames()
		for _, key := range []string{"branch." + head.Name + ".pushRemote", "remote.pushDefault"} {
			name, err := r.git_("config", "--get", key)
			if err != nil {
				continue // not set
			}
			if name = strings.TrimSpace(name); !slices.Contains(remotes, name) {
				return answer{}, fmt.Errorf("%s is %q, which isn't one of the repository's remotes", key, name)
			}
			return answer{remote: name}, nil
		}
		switch {
		case len(remotes) == 0:
			return answer{}, nil
		case slices.Contains(remotes, "origin"):
			return answer{"origin", len(remotes) > 1}, nil
		}
		return answer{remotes[0], len(remotes) > 1}, nil
	})
	return a.remote, a.ask, err
}

// remoteNames lists the repository's remotes, one name a line, each whole,
// as git config can make one with a space in it; it must be current.
func (r *Repo) remoteNames() []string {
	out, _ := r.git_("remote")
	var names []string
	for _, name := range strings.Split(out, "\n") {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// Push pushes the branch checked out. One with no upstream yet is pushed
// to remote, which becomes its upstream (see PushRemote).
func (r *Repo) Push(head *models.Branch, remote string, force bool) error {
	if head == nil || head.DetachedHead {
		return errors.New("check out a branch to push it")
	}
	if err := checkRef(head.Name); err != nil {
		return err
	}
	opts := git_commands.PushOpts{ForceWithLease: force}
	if !head.IsTrackingRemote() {
		// git push takes its remote where its options go, with no "--"
		// before it: a remote named "--force" would be the option.
		if remote == "" {
			return errors.New("the repository has no remote to push to")
		}
		if strings.HasPrefix(remote, "-") {
			return fmt.Errorf("%q isn't a remote laziergit can push to", remote)
		}
		opts.SetUpstream = true
		opts.Remote = remote
		opts.Refspecs = []string{"refs/heads/" + head.Name + ":refs/heads/" + head.Name}
	}
	return r.do(func(g *commands.GitCommand) error {
		if opts.Remote != "" && !slices.Contains(r.remoteNames(), opts.Remote) {
			return fmt.Errorf("the repository has no remote %q", opts.Remote)
		}
		return g.Sync.Push(gocui.NewFakeTask(), opts)
	})
}

// Stash

func (r *Repo) StashAll(message string) error {
	return r.do(func(g *commands.GitCommand) error { return g.Stash.Push(message) })
}

// StashPop applies a stash entry, then drops it. Applying goes by the
// entry's commit, which nothing changes; dropping can only go by its index
// in the stash, which is checked again just before (see stashDo).
func (r *Repo) StashPop(e *models.StashEntry) error {
	return r.stashDo(e, func(g *commands.GitCommand) error {
		if err := r.gitRun("stash", "apply", e.Hash); err != nil {
			return err // as git stash pop does, the entry stays
		}
		if err := r.checkStash(e); err != nil {
			return staleError("applied, but " + e.RefName() + " is no longer the stash shown, so it is kept: look at the stashes again")
		}
		if err := r.dropStash(e); err != nil {
			return fmt.Errorf("applied, but not dropped: %w", err)
		}
		return nil
	})
}

// reDropped finds the commit of the entry git stash drop says it dropped.
var reDropped = regexp.MustCompile(`\(([0-9a-f]{40}|[0-9a-f]{64})\)\s*$`)

// dropStash drops e by its index, the only way git has, then makes sure
// the entry dropped was e: if it was another, its index e's no longer,
// as by a stash made since the check by a git command outside the app,
// that entry is stored back, and dropStash fails with ErrStale.
//
// git's words are read in English, whatever the user's language. The entry
// put back goes on top, named by its commit's subject, which is its name
// in the stash unless that was set otherwise.
func (r *Repo) dropStash(e *models.StashEntry) error {
	out, err := r.osCmd.Cmd.New([]string{"git", "stash", "drop", e.FullRefName()}).
		AddEnvVars("LC_ALL=C", "LANGUAGE=C").RunWithOutput()
	if err != nil {
		return err
	}
	m := reDropped.FindStringSubmatch(strings.TrimSpace(out))
	if m != nil && m[1] == e.Hash {
		return nil
	}
	if m == nil {
		// git's words didn't say which: e gone from the stash tells.
		list, err := r.git_("log", "-g", "--format=%H", "refs/stash")
		if err == nil && !slices.Contains(strings.Fields(list), e.Hash) {
			return nil
		}
		return staleError("git didn't say which stash it dropped, nor is it sure " + e.RefName() + " was: look at the stashes again")
	}
	subject, err := r.git_("log", "-1", "--format=%s", m[1])
	if err == nil {
		err = r.gitRun("stash", "store", "-m", strings.TrimSpace(subject), m[1])
	}
	if err != nil {
		return fmt.Errorf("%s was another stash's when dropped, %s, which couldn't be put back (git stash store %s does): %w", e.RefName(), m[1], m[1], err)
	}
	return staleError(e.RefName() + " was another stash's when dropped, which is put back on top: look at the stashes again")
}

func (r *Repo) StashApply(e *models.StashEntry) error {
	return r.stashDo(e, func(g *commands.GitCommand) error { return r.gitRun("stash", "apply", e.Hash) })
}

func (r *Repo) StashDrop(e *models.StashEntry) error {
	return r.stashDo(e, func(g *commands.GitCommand) error { return r.dropStash(e) })
}

// stashDo runs fn on the stash entry e, failing with ErrStale when its
// index is another entry's now, as after a stash made since. An index
// can change between the check and fn's command still, by a git command
// outside the app, which the lock doesn't keep out: fn goes by e's commit
// where git lets it, and dropStash makes sure of what it dropped.
func (r *Repo) stashDo(e *models.StashEntry, fn func(g *commands.GitCommand) error) error {
	return r.do(func(g *commands.GitCommand) error {
		if err := r.checkStash(e); err != nil {
			return err
		}
		return fn(g)
	})
}

// checkStash fails with ErrStale when e's index is another entry's now.
func (r *Repo) checkStash(e *models.StashEntry) error {
	now, err := r.osCmd.Cmd.New([]string{"git", "rev-parse", "--verify", "--quiet", e.FullRefName()}).DontLog().RunWithOutput()
	if err != nil || e.Hash == "" || strings.TrimSpace(now) != e.Hash {
		return staleError(e.RefName() + " is no longer the stash shown: look at the stashes again")
	}
	return nil
}
