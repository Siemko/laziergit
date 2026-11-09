package core

import (
	"errors"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/gocui"
	"github.com/stefanhaller/git-todo-parser/todo"
)

// Files

func (r *Repo) ToggleStaged(f *models.File) error {
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

func (r *Repo) Commit(summary, description string, amend bool) error {
	if summary == "" && !amend {
		return errors.New("the commit needs a summary")
	}
	return r.do(func(g *commands.GitCommand) error {
		if amend {
			return g.Commit.AmendHead()
		}
		return g.Commit.CommitCmdObj(summary, description, false).Run()
	})
}

// The commit actions below rewrite history with an interactive rebase,
// whose todo list lazygit's daemon (see HandleDaemon) edits.

func (r *Repo) DropCommit(commits []*models.Commit, i int) error {
	return r.do(func(g *commands.GitCommand) error {
		return g.Rebase.InteractiveRebase(commits, i, i, todo.Drop, "")
	})
}

// FixupCommit squashes a commit into the one below it, keeping that one's
// message.
func (r *Repo) FixupCommit(commits []*models.Commit, i int) error {
	if i+1 >= len(commits) {
		return errors.New("there is no commit below to fix up into")
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.git.Commit.GetCommitMessage(c.Hash())
}

// Branches

func (r *Repo) Checkout(name string) error {
	return r.do(func(g *commands.GitCommand) error {
		return g.Branch.Checkout(name, git_commands.CheckoutOptions{})
	})
}

func (r *Repo) NewBranch(name, base string) error {
	return r.do(func(g *commands.GitCommand) error { return g.Branch.New(name, base) })
}

func (r *Repo) DeleteBranch(name string, force bool) error {
	return r.do(func(g *commands.GitCommand) error { return g.Branch.LocalDelete([]string{name}, force) })
}

func (r *Repo) Merge(name string) error {
	return r.do(func(g *commands.GitCommand) error {
		return g.Branch.Merge(name, git_commands.MERGE_VARIANT_REGULAR)
	})
}

func (r *Repo) RebaseOnto(name string) error {
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
	opts := git_commands.PushOpts{ForceWithLease: force}
	if head != nil && !head.IsTrackingRemote() {
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
