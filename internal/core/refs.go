package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gocui"
)

// Tags, remote branches, the reflog, worktrees and submodules: what a
// snapshot holds of them besides the branches and the log, and the
// actions on them.

// ReflogLimit is how many entries of the reflog a snapshot loads.
const ReflogLimit = 300

// Submodule is a submodule of the repository, as .gitmodules lists it,
// with its state as git submodule status tells it.
type Submodule struct {
	*models.SubmoduleConfig
	// Status is git's: ' ' at the commit recorded, '-' not initialized,
	// '+' at another commit, 'U' with merge conflicts.
	Status byte
	// Head is the commit checked out in it, or recorded for one not
	// initialized.
	Head string
}

// Initialized reports whether the submodule's work tree is checked out.
func (s *Submodule) Initialized() bool { return s.Status != '-' }

// loadMore fills in what the snapshot holds besides the branches and the
// log. Each is optional: a list git fails to give, as worktrees in a
// repository too old for them, is empty rather than failing the load.
func (r *Repo) loadMore(s *Snapshot) {
	l := r.git.Loaders
	r.timed("tags", func() {
		if tags, err := l.TagLoader.GetTags(); err == nil {
			s.Tags = tags
		}
	})
	r.timed("remote branches", func() {
		if byRemote, err := l.RemoteLoader.GetRemoteBranchesByRemoteName(); err == nil {
			for _, remote := range s.Remotes {
				remote.Branches = byRemote[remote.Name]
			}
		}
	})
	r.timed("reflog", func() {
		if reflog, err := r.reflog(ReflogLimit); err == nil {
			s.Reflog = reflog
		}
	})
	r.timed("worktrees", func() {
		if worktrees, err := l.Worktrees.GetWorktrees(); err == nil {
			s.Worktrees = worktrees
		}
	})
	r.timed("submodules", func() { s.Submodules = r.submodules() })
}

// reflog is the latest limit entries of HEAD's reflog, newest first, as
// lazygit's reflog loader makes them; it reads them alone, rather than
// the whole reflog, which a repository used for years has tens of
// thousands of.
func (r *Repo) reflog(limit int) ([]*models.Commit, error) {
	out, err := r.osCmd.Cmd.New([]string{"git", "-c", "log.showSignature=false", "log", "-g",
		"-n", strconv.Itoa(limit), "--format=%H%x00%ct%x00%gs%x00%P"}).DontLog().RunWithOutput()
	if err != nil {
		return nil, err
	}
	var commits []*models.Commit
	for _, line := range strings.Split(out, "\n") {
		fields := strings.SplitN(line, "\x00", 4)
		if len(fields) < 4 {
			continue
		}
		when, _ := strconv.ParseInt(fields[1], 10, 64)
		parents := []string{}
		if fields[3] != "" {
			parents = strings.Split(fields[3], " ")
		}
		commits = append(commits, models.NewCommit(r.hashPool, models.NewCommitOpts{
			Hash: fields[0], Name: fields[2], UnixTimestamp: when, Status: models.StatusReflog, Parents: parents,
		}))
	}
	return commits, nil
}

// submodules lists the submodules .gitmodules declares, with their state.
func (r *Repo) submodules() []*Submodule {
	configs, err := r.git.Submodule.GetConfigs(nil)
	if err != nil || len(configs) == 0 {
		return nil
	}
	status := map[string][2]string{}
	if out, err := r.git_("submodule", "status"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if len(line) < 2 {
				continue
			}
			// "<state><hash> <path> (<describe>)"
			fields := strings.Fields(line[1:])
			if len(fields) >= 2 {
				status[fields[1]] = [2]string{line[:1], fields[0]}
			}
		}
	}
	out := make([]*Submodule, 0, len(configs))
	for _, c := range configs {
		sm := &Submodule{SubmoduleConfig: c, Status: '-'}
		if st, ok := status[c.Path]; ok {
			sm.Status, sm.Head = st[0][0], st[1]
		}
		out = append(out, sm)
	}
	return out
}

// RefCommits loads the latest commits of a ref, by its full name, as
// refs/tags/v1, with their graph.
func (r *Repo) RefCommits(fullRef string, limit int) ([]*models.Commit, [][]Pipe, error) {
	if err := checkRef(fullRef); err != nil {
		return nil, nil, err
	}
	leave, err := r.enter()
	if err != nil {
		return nil, nil, err
	}
	defer leave()
	commits, err := r.git.Loaders.CommitLoader.GetCommits(git_commands.GetCommitsOptions{
		Limit:        limit,
		RefName:      fullRef,
		MainBranches: r.mainBranches,
		HashPool:     r.hashPool,
	})
	if err != nil {
		return nil, nil, err
	}
	return commits, GraphRows(commits), nil
}

// checkRemote fails for a remote git would read as an option, or one the
// repository doesn't have, which git would take for a path or a URL. The
// repository must be current.
func (r *Repo) checkRemote(name string) error {
	if name == "" || strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q isn't a remote laziergit can use", name)
	}
	if !slices.Contains(r.remoteNames(), name) {
		return fmt.Errorf("the repository has no remote %q", name)
	}
	return nil
}

// Tags

// CreateTag tags ref: an annotated tag with message, or a lightweight one
// without.
func (r *Repo) CreateTag(name, ref, message string) error {
	if err := errors.Join(checkRef(name), checkRef(ref)); err != nil {
		return err
	}
	args := []string{"tag"}
	if message != "" {
		args = append(args, "-a", "-m", message)
	}
	args = append(args, "--", name, ref)
	return r.do(func(g *commands.GitCommand) error { return r.gitRun(args...) })
}

func (r *Repo) DeleteTag(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return g.Tag.LocalDelete([]string{name}) })
}

// PushTag pushes a tag to a remote.
func (r *Repo) PushTag(remote, name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		if err := r.checkRemote(remote); err != nil {
			return err
		}
		return g.Tag.Push(gocui.NewFakeTask(), remote, []string{name})
	})
}

// CheckoutRef checks out a tag or a commit, by its full name or hash,
// detaching the head.
func (r *Repo) CheckoutRef(ref string) error {
	if err := checkRef(ref); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return r.checkout(ref, nil) })
}

// Remote branches

// CheckoutRemoteBranch checks out the local branch of a remote branch's
// name, making it, tracking the remote one, when there is none.
func (r *Repo) CheckoutRemoteBranch(b *models.RemoteBranch) error {
	if err := errors.Join(checkRef(b.Name), checkRef(b.RemoteName)); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		if !r.branchExists(b.Name) {
			if err := r.gitRun("branch", "--track", "--", b.Name, b.FullRefName()); err != nil {
				return err
			}
		}
		return r.checkout(b.Name, nil)
	})
}

// DeleteRemoteBranch deletes a branch on its remote.
func (r *Repo) DeleteRemoteBranch(b *models.RemoteBranch) error {
	if err := checkRef(b.Name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		if err := r.checkRemote(b.RemoteName); err != nil {
			return err
		}
		return g.Remote.DeleteRemoteBranch(gocui.NewFakeTask(), b.RemoteName, []string{b.Name})
	})
}

// FetchRemote fetches one remote.
func (r *Repo) FetchRemote(remote string) error {
	return r.do(func(g *commands.GitCommand) error {
		if err := r.checkRemote(remote); err != nil {
			return err
		}
		return g.Sync.FetchRemote(gocui.NewFakeTask(), remote)
	})
}

// Reflog

// ResetMode is how ResetTo resets: soft keeps the index and the work tree,
// mixed the work tree, hard neither.
type ResetMode string

const (
	ResetSoft  ResetMode = "soft"
	ResetMixed ResetMode = "mixed"
	ResetHard  ResetMode = "hard"
)

// ResetTo resets the branch checked out to a commit. A hard reset refuses
// to overwrite files git doesn't track, and changes git stash would pass
// over, as an undo's does; the changes to tracked files are lost, which
// the UI asks about.
func (r *Repo) ResetTo(hash string, mode ResetMode) error {
	if err := checkRef(hash); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		switch mode {
		case ResetSoft, ResetMixed:
			return g.Commit.ResetToCommit(hash, string(mode), nil)
		case ResetHard:
			if err := r.checkUntrackedAgainst(hash); err != nil {
				return err
			}
			if err := r.checkAssumedUnchanged(); err != nil {
				return err
			}
			return r.hardReset(hash, nil)
		}
		return fmt.Errorf("no reset %q", mode)
	})
}

// Worktrees

// ErrWorktreeDirty is the error of removing, without forcing, a worktree
// with changes, which are lost with it.
var ErrWorktreeDirty = errors.New("the worktree has changes")

// WorktreePath is where a path typed for a new worktree goes: relative to
// the repository's work tree, so that "../app-fix" is beside it.
func (r *Repo) WorktreePath(path string) string {
	if path = strings.TrimSpace(path); path == "" {
		return ""
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.Path, path)
	}
	return filepath.Clean(path)
}

// AddWorktree makes a worktree at path, checking out base, or, with
// branch, a new branch of that name from base.
func (r *Repo) AddWorktree(path, base, branch string) error {
	path = r.WorktreePath(path)
	if path == "" {
		return errors.New("the worktree needs a folder")
	}
	if err := checkRef(base); err != nil {
		return err
	}
	if branch != "" {
		if err := checkRef(branch); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(path); err == nil && holdsFiles(path) {
		return fmt.Errorf("%s isn't empty", path)
	}
	return r.do(func(g *commands.GitCommand) error {
		return g.Worktree.New(git_commands.NewWorktreeOpts{Path: path, Base: base, Branch: branch})
	})
}

// RemoveWorktree removes a linked worktree: unless force, only one without
// changes, failing with ErrWorktreeDirty otherwise.
func (r *Repo) RemoveWorktree(w *models.Worktree, force bool) error {
	switch {
	case w.IsMain:
		return errors.New("the main worktree can't be removed")
	case w.IsCurrent:
		return errors.New("the worktree open here can't be removed: open another")
	case !filepath.IsAbs(w.Path):
		return fmt.Errorf("%q isn't a worktree's path", w.Path)
	}
	return r.do(func(g *commands.GitCommand) error {
		if !force && !w.IsPathMissing {
			out, err := r.git_("-C", w.Path, "status", "--porcelain", "--ignore-submodules=none")
			if err != nil {
				return fmt.Errorf("reading the worktree's status: %w", err)
			}
			if strings.TrimSpace(out) != "" {
				return ErrWorktreeDirty
			}
		}
		return g.Worktree.Delete(w.Path, force)
	})
}

// Submodules

// checkSubmodulePath fails for a submodule path outside the work tree, or
// one git would read as an option.
func checkSubmodulePath(path string) error {
	if path == "" || strings.HasPrefix(path, "-") || filepath.IsAbs(path) || !filepath.IsLocal(path) {
		return fmt.Errorf("%q isn't a submodule's path", path)
	}
	return nil
}

// UpdateSubmodule initializes a submodule if need be, and checks out the
// commit the repository records for it.
func (r *Repo) UpdateSubmodule(s *Submodule) error {
	if err := checkSubmodulePath(s.Path); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		return r.gitRun("submodule", "update", "--init", "--", s.Path)
	})
}

// UpdateAllSubmodules does UpdateSubmodule for every one.
func (r *Repo) UpdateAllSubmodules() error {
	return r.do(func(g *commands.GitCommand) error { return r.gitRun("submodule", "update", "--init") })
}

// AddSubmodule adds the repository at url as a submodule at path.
func (r *Repo) AddSubmodule(url, path string) error {
	url, path = strings.TrimSpace(url), strings.TrimSpace(path)
	if url == "" || strings.HasPrefix(url, "-") {
		return fmt.Errorf("%q isn't a repository's URL", url)
	}
	if path == "" {
		path = strings.TrimSuffix(filepath.Base(strings.TrimRight(url, "/")), ".git")
	}
	if err := checkSubmodulePath(path); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		return r.gitRun("submodule", "add", "--", url, path)
	})
}

// RemoveSubmodule removes a submodule: its entry, its work tree, and its
// repository in .git/modules.
func (r *Repo) RemoveSubmodule(s *Submodule) error {
	if err := checkSubmodulePath(s.Path); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return g.Submodule.Delete(s.SubmoduleConfig) })
}
