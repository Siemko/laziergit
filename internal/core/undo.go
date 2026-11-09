package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

// The undo and redo below are lazygit's (pkg/gui/controllers/undo_controller.go):
// they walk the reflog, and mark what they do in it as lazygit does, so
// that undoing in either app goes on from the other's steps. They also undo
// what lazygit's walk passes over: cherry-picks and reverts, as commits,
// and merges, as rebases.

// UndoKind is what kind of thing a step of the reflog did.
type UndoKind int

const (
	// UndoCommit is a commit, a reset or a pull: HEAD moved on its branch.
	UndoCommit UndoKind = iota
	// UndoRebase is a whole rebase, from its start to its finish or abort.
	UndoRebase
	// UndoCheckout is a checkout of another branch or commit.
	UndoCheckout
	// UndoMerge is a merge, undone, as a rebase, with a hard reset.
	UndoMerge
)

// UndoStep is a step of the reflog that undo or redo would take back or
// do again: HEAD went From one place To another.
type UndoStep struct {
	Kind     UndoKind
	From, To string
}

// Describe says what undoing (or redoing) the step does.
func (s UndoStep) Describe(redo bool) string {
	target := s.From
	if redo {
		target = s.To
	}
	switch {
	case s.Kind == UndoCheckout:
		return "Check out " + target + "."
	case s.Kind == UndoCommit && !redo:
		return "Soft reset to " + short(target) + ": the commit's changes stay, staged."
	}
	return "Hard reset to " + short(target) + ", stashing local changes around it."
}

func short(hash string) string { return hash[:min(len(hash), 8)] }

const (
	undoAction = "[lazygit undo]"
	redoAction = "[lazygit redo]"
)

var (
	reUndo     = regexp.MustCompile(`^\[lazygit undo\]`)
	reRedo     = regexp.MustCompile(`^\[lazygit redo\]`)
	reFinish   = regexp.MustCompile(`^rebase (-i )?\(abort\)|^rebase (-i )?\(finish\)`)
	reCheckout = regexp.MustCompile(`^checkout: moving from ([\S]+) to ([\S]+)`)
	reCommit   = regexp.MustCompile(`^commit|^reset: moving to|^pull|^cherry-pick|^revert`)
	reMerge    = regexp.MustCompile(`^merge `)
	reStart    = regexp.MustCompile(`^rebase (-i )?\(start\)`)
)

// errMidRebase is undoing inside a rebase, which needs the todo's history.
var errMidRebase = errors.New("can't undo or redo in the middle of a rebase")

// FindUndo finds the step undo (or redo) would take, walking the reflog as
// lazygit does: each undo counts one step back, each redo one forward.
func (r *Repo) FindUndo(redo bool) (UndoStep, error) {
	leave, err := r.enter()
	if err != nil {
		return UndoStep{}, err
	}
	defer leave()
	if r.git.Status.WorkingTreeState().Any() {
		return UndoStep{}, errMidRebase
	}
	reflog, _, err := r.git.Loaders.ReflogCommitLoader.GetReflogCommits(r.hashPool, nil, "", "")
	if err != nil {
		return UndoStep{}, err
	}
	return findUndo(reflog, redo)
}

// undoCurrentRebase marks the start of a rebase that hasn't finished.
const undoCurrentRebase UndoKind = -1

func findUndo(reflog []*models.Commit, redo bool) (UndoStep, error) {
	// pick decides on each step, newest first, counting the steps undone
	// already: undo takes the first not undone yet, redo the last undone.
	pick := func(counter int, s UndoStep) (done bool, err error) {
		if !redo {
			if counter != 0 {
				return false, nil
			}
		} else {
			switch {
			case counter == 0:
				return true, errors.New("there is nothing to redo")
			case counter > 1:
				return false, nil
			}
		}
		if s.Kind == undoCurrentRebase {
			return true, errMidRebase
		}
		return true, nil
	}
	counter := 0
	rebaseFinish := ""
	for i, c := range reflog {
		prev := ""
		if i+1 < len(reflog) {
			prev = reflog[i+1].Hash()
		}
		var step *UndoStep
		if rebaseFinish == "" {
			switch {
			case reUndo.MatchString(c.Name):
				counter++
			case reRedo.MatchString(c.Name):
				counter--
			case reFinish.MatchString(c.Name):
				rebaseFinish = c.Hash()
			case reCheckout.MatchString(c.Name):
				m := reCheckout.FindStringSubmatch(c.Name)
				step = &UndoStep{Kind: UndoCheckout, From: m[1], To: m[2]}
			case reCommit.MatchString(c.Name):
				step = &UndoStep{Kind: UndoCommit, From: prev, To: c.Hash()}
			case reMerge.MatchString(c.Name):
				step = &UndoStep{Kind: UndoMerge, From: prev, To: c.Hash()}
			case reStart.MatchString(c.Name):
				step = &UndoStep{Kind: undoCurrentRebase, From: prev}
			}
		} else if reStart.MatchString(c.Name) {
			step = &UndoStep{Kind: UndoRebase, From: prev, To: rebaseFinish}
			rebaseFinish = ""
		}
		if step == nil || step.Kind != undoCurrentRebase && step.From == step.To {
			continue
		}
		if done, err := pick(counter, *step); done {
			return *step, err
		}
		counter--
	}
	if redo {
		return UndoStep{}, errors.New("there is nothing to redo")
	}
	return UndoStep{}, errors.New("there is nothing to undo")
}

// ErrStashKept wraps the error of an undo whose local changes, stashed
// around its reset, didn't apply again: they are in stash@{0}.
var ErrStashKept = errors.New("your local changes are kept in stash@{0}")

// ApplyUndo undoes (or redoes) a step FindUndo found, marking it in the
// reflog. A hard reset stashes the work tree's changes around it, as
// lazygit's does, and gives back only the stash it made; it refuses to
// overwrite files git doesn't track (untracked or ignored) where the
// target has files.
func (r *Repo) ApplyUndo(s UndoStep, redo bool) error {
	env := []string{"GIT_REFLOG_ACTION=" + undoAction}
	target := s.From
	if redo {
		env = []string{"GIT_REFLOG_ACTION=" + redoAction}
		target = s.To
	}
	if err := checkRef(target); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		switch {
		case s.Kind == UndoCheckout:
			return g.Branch.Checkout(target, git_commands.CheckoutOptions{EnvVars: env})
		case s.Kind == UndoCommit && !redo:
			return g.Commit.ResetToCommit(target, "soft", env)
		}
		if err := r.checkUntrackedAgainst(target); err != nil {
			return err
		}
		// The stash is always asked for, rather than when the status says
		// the tree is dirty: a status that failed or hides files would
		// lose them. It saves nothing for a clean tree, which refs/stash
		// tells.
		before := r.stashTop()
		if err := g.Stash.Push(fmt.Sprintf("laziergit auto-stash for undo to %s", short(target))); err != nil {
			return fmt.Errorf("stashing local changes before the reset: %w", err)
		}
		stashed := r.stashTop()
		made := stashed != "" && stashed != before
		if err := g.Commit.ResetToCommit(target, "hard", env); err != nil {
			if made {
				_ = g.Stash.Pop(0)
			}
			return err
		}
		if !made {
			return nil
		}
		if err := g.Stash.Pop(0); err != nil {
			return fmt.Errorf("%w (%s), which didn't apply cleanly: %w", ErrStashKept, short(stashed), err)
		}
		return nil
	})
}

// checkUntrackedAgainst fails when files git doesn't track, untracked or
// ignored, are where target has files, which a hard reset to it would
// overwrite or delete: a local file at a file's path or at a directory of
// one, local files in a directory the target has a file at, and, in a
// local directory of such files, a path the target writes that is there
// already.
func (r *Repo) checkUntrackedAgainst(target string) error {
	// --directory lists a directory of untracked files as itself, with a
	// slash; without --exclude-standard, ignored files are listed too.
	out, err := r.git_("ls-files", "--others", "--directory", "-z")
	if err != nil {
		return fmt.Errorf("listing untracked files: %w", err)
	}
	var local []string
	isFile, isDir := map[string]bool{}, map[string]bool{}
	for _, p := range strings.Split(out, "\x00") {
		switch {
		case p == "":
		case strings.HasSuffix(p, "/"):
			p = strings.TrimSuffix(p, "/")
			isDir[p] = true
			local = append(local, p)
		default:
			isFile[p] = true
			local = append(local, p)
		}
	}
	if len(local) == 0 {
		return nil
	}
	slices.Sort(local)
	tree, err := r.git_("ls-tree", "-r", "-z", "--name-only", target)
	if err != nil {
		return err
	}
	index, err := r.git_("ls-files", "-z")
	if err != nil {
		return err
	}
	inIndex := map[string]bool{}
	for _, p := range strings.Split(index, "\x00") {
		inIndex[p] = true
	}
	var hit []string
	for _, p := range strings.Split(tree, "\x00") {
		if p == "" {
			continue
		}
		// A path the reset creates that the file system has already, by
		// another name it takes for the same, as "docs/a.md" for
		// "Docs/A.md" on a disk that ignores case.
		if !inIndex[p] {
			if st, err := os.Lstat(filepath.Join(r.Path, p)); err == nil && !st.IsDir() && !isFile[p] && !isDir[p] {
				if !r.trackedAs(p, inIndex) {
					hit = append(hit, p)
					continue
				}
			}
		}
		for dir := p; dir != "." && dir != "/"; dir = filepath.Dir(dir) {
			if isFile[dir] {
				hit = append(hit, dir) // a local file where the target has a file or a directory
				break
			}
			if isDir[dir] {
				// A directory of local files at the path itself, which the
				// reset would replace by a file; or above it, which the
				// reset writes into, overwriting only what is in its way.
				if dir == p || r.inTheWay(dir, p) {
					hit = append(hit, p)
				}
				break
			}
		}
		// Local files under a path the target has a file at.
		if i, _ := slices.BinarySearch(local, p+"/"); i < len(local) && strings.HasPrefix(local[i], p+"/") {
			hit = append(hit, local[i])
		}
	}
	if len(hit) > 0 {
		hit = slices.Compact(hit)
		return fmt.Errorf("files git doesn't track would be overwritten: %s; move them away first", strings.Join(hit[:min(len(hit), 5)], ", "))
	}
	return nil
}

// trackedAs reports whether the file at p on disk is a tracked file under
// another spelling of its name, as on a disk that ignores case, which the
// reset rewrites rather than loses: the same file as one the index has.
func (r *Repo) trackedAs(p string, inIndex map[string]bool) bool {
	st, err := os.Stat(filepath.Join(r.Path, p))
	if err != nil {
		return false
	}
	dir := filepath.Dir(p)
	entries, err := os.ReadDir(filepath.Join(r.Path, dir))
	if err != nil {
		return false
	}
	for _, e := range entries {
		q := filepath.Join(dir, e.Name())
		if dir == "." {
			q = e.Name()
		}
		if !inIndex[q] {
			continue
		}
		if other, err := os.Stat(filepath.Join(r.Path, q)); err == nil && os.SameFile(st, other) {
			return true
		}
	}
	return false
}

// inTheWay reports whether writing the file p, under the local directory
// dir, meets something there: the file itself, or a file where p needs a
// directory.
func (r *Repo) inTheWay(dir, p string) bool {
	for q := p; q != dir && q != "." && q != "/"; q = filepath.Dir(q) {
		st, err := os.Lstat(filepath.Join(r.Path, q))
		if err != nil {
			continue
		}
		if q == p || !st.IsDir() {
			return true
		}
	}
	return false
}

// stashTop is the commit refs/stash points at, empty for no stash.
func (r *Repo) stashTop() string {
	out, err := r.git_("rev-parse", "-q", "--verify", "refs/stash")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// git_ runs git in the repository, which must be current.
func (r *Repo) git_(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", r.Path}, args...)...)
	out, err := cmd.Output()
	return string(out), err
}

// BranchCommits loads the latest commits of a branch, with their graph.
func (r *Repo) BranchCommits(b *models.Branch, limit int) ([]*models.Commit, [][]Pipe, error) {
	leave, err := r.enter()
	if err != nil {
		return nil, nil, err
	}
	defer leave()
	commits, err := r.git.Loaders.CommitLoader.GetCommits(git_commands.GetCommitsOptions{
		Limit:              limit,
		RefName:            b.FullRefName(),
		RefForPushedStatus: b,
		MainBranches:       r.mainBranches,
		HashPool:           r.hashPool,
	})
	if err != nil {
		return nil, nil, err
	}
	return commits, GraphRows(commits), nil
}

// CherryPick copies commits onto HEAD, given newest first as logs list
// them; they are applied oldest first.
func (r *Repo) CherryPick(commits []*models.Commit) error {
	if len(commits) == 0 {
		return errors.New("no commits to cherry-pick")
	}
	return r.doMayConflict(func(g *commands.GitCommand) error { return g.Rebase.CherryPickCommits(commits) })
}
