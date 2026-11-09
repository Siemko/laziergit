package core

import (
	"errors"
	"fmt"
	"strings"

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
			return g.WorkingTree.StageFile(f.Path)
		}
		return g.WorkingTree.UnStageFile(f.Names(), f.Tracked)
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
		return g.WorkingTree.StageAll(false)
	})
}

func (r *Repo) Discard(f *models.File) error {
	return r.do(func(g *commands.GitCommand) error { return g.WorkingTree.DiscardAllFileChanges(f) })
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
	return r.do(func(g *commands.GitCommand) error {
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
	return r.do(func(g *commands.GitCommand) error {
		return g.Rebase.InteractiveRebase(commits, i, i, todo.Fixup, "")
	})
}

func (r *Repo) MoveCommit(commits []*models.Commit, i, delta int) error {
	return r.do(func(g *commands.GitCommand) error {
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
	return r.do(func(g *commands.GitCommand) error {
		return g.Branch.Checkout(name, git_commands.CheckoutOptions{})
	})
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

// DeleteBranch deletes a branch: unless force, only one merged, failing
// with ErrNotMerged otherwise.
func (r *Repo) DeleteBranch(name string, force bool) error {
	if err := checkRef(name); err != nil {
		return err
	}
	err := r.do(func(g *commands.GitCommand) error { return g.Branch.LocalDelete([]string{name}, force) })
	if err != nil && !force && strings.Contains(err.Error(), "not fully merged") {
		return ErrNotMerged
	}
	return err
}

func (r *Repo) Merge(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error {
		return g.Branch.Merge(name, git_commands.MERGE_VARIANT_REGULAR)
	})
}

func (r *Repo) RebaseOnto(name string) error {
	if err := checkRef(name); err != nil {
		return err
	}
	return r.do(func(g *commands.GitCommand) error { return g.Rebase.RebaseBranch(name) })
}

// Remotes

func (r *Repo) Fetch() error {
	return r.do(func(g *commands.GitCommand) error { return g.Sync.Fetch(gocui.NewFakeTask()) })
}

func (r *Repo) Pull() error {
	return r.do(func(g *commands.GitCommand) error {
		return g.Sync.Pull(gocui.NewFakeTask(), git_commands.PullOptions{})
	})
}

// Push pushes the branch checked out, setting its upstream on the first
// remote when it has none.
func (r *Repo) Push(head *models.Branch, remotes []*models.Remote, force bool) error {
	if head == nil || head.DetachedHead {
		return errors.New("check out a branch to push it")
	}
	if err := checkRef(head.Name); err != nil {
		return err
	}
	opts := git_commands.PushOpts{ForceWithLease: force}
	if !head.IsTrackingRemote() {
		if len(remotes) == 0 {
			return errors.New("the repository has no remote to push to")
		}
		opts.SetUpstream = true
		opts.Remote = remotes[0].Name
		opts.Refspecs = []string{"refs/heads/" + head.Name + ":refs/heads/" + head.Name}
	}
	return r.do(func(g *commands.GitCommand) error { return g.Sync.Push(gocui.NewFakeTask(), opts) })
}

// Stash

func (r *Repo) StashAll(message string) error {
	return r.do(func(g *commands.GitCommand) error { return g.Stash.Push(message) })
}

func (r *Repo) StashPop(e *models.StashEntry) error {
	return r.do(func(g *commands.GitCommand) error { return g.Stash.Pop(e.Index) })
}

func (r *Repo) StashApply(e *models.StashEntry) error {
	return r.do(func(g *commands.GitCommand) error { return g.Stash.Apply(e.Index) })
}

func (r *Repo) StashDrop(e *models.StashEntry) error {
	return r.do(func(g *commands.GitCommand) error { return g.Stash.Drop(e.Index) })
}
