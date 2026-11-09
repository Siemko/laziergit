// Package core drives a git repository through lazygit's command layer
// (github.com/jesseduffield/lazygit/pkg/commands), for a UI to show and act on.
package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/jesseduffield/lazygit/pkg/app/daemon"
	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_config"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/commands/oscommands"
	"github.com/jesseduffield/lazygit/pkg/common"
	"github.com/jesseduffield/lazygit/pkg/config"
	"github.com/jesseduffield/lazygit/pkg/env"
	"github.com/jesseduffield/lazygit/pkg/i18n"
	"github.com/jesseduffield/lazygit/pkg/utils"
	"github.com/sirupsen/logrus"
	"github.com/spf13/afero"
)

// HandleDaemon runs the process as lazygit's daemon when git started it so,
// as the sequence editor of an interactive rebase, and reports whether it
// did. main must call it before anything else: lazygit's rebases re-run
// the app's own binary.
func HandleDaemon() bool {
	if !daemon.InDaemonMode() {
		return false
	}
	log := logrus.New()
	log.SetOutput(io.Discard)
	daemon.Handle(logrus.NewEntry(log))
	return true
}

// Credential is what a command asks the user for.
type Credential = oscommands.CredentialType

// Options are the hooks a UI gives a repository.
type Options struct {
	// ConfigDir holds lazygit's config.yml and state.yml for the app; empty
	// keeps lazygit's own lookup (CONFIG_DIR, then lazygit's directory).
	ConfigDir string
	// OnCommand is told of each command about to run, from any goroutine.
	OnCommand func(cmd string, commandLine bool)
	// AskCredential asks for a username, password or passphrase a command
	// waits on; the channel gives the answer. From any goroutine.
	AskCredential func(kind Credential) <-chan string
}

// Repo is an open repository. Its methods run git and block; call them off
// the UI thread.
//
// Several repositories can be open at once, one per window. lazygit's
// commands assume one repository per process, though: it changes the
// process's working directory and git's environment for it, and some of
// its file operations go by paths relative to the working directory. So
// the repositories take turns: one command runs at a time, in the whole
// process, with its repository current (see enter).
type Repo struct {
	cmn          *common.Common
	osCmd        *oscommands.OSCommand
	git          *commands.GitCommand
	mainBranches *git_commands.MainBranches
	hashPool     *utils.StringPool
	tempDir      string
	closed       bool // under gitMu
	// closing is set as soon as the repository is to close: commands that
	// haven't started yet fail, without waiting for the lock.
	closing atomic.Bool

	// Path is the work tree's root.
	Path string
}

var (
	// gitMu is held while a command of any repository runs.
	gitMu sync.Mutex
	// current is the repository the working directory and git's
	// environment are set for, under gitMu.
	current *Repo
)

// literalPaths makes git read the paths of the commands fn runs as file
// names, never patterns: a file named "app/[slug]/page.tsx" or ":(glob)*"
// stands for itself, not for others it would match. The variable is set
// only while fn runs, under gitMu, around which every git command of the
// app runs, rather than for the whole process: git hands it on to hooks,
// whose own patterns would stop matching. The commands given paths (add,
// reset, checkout of files, diff, apply) run no commit hooks.
func literalPaths(fn func() error) error {
	old, had := os.LookupEnv("GIT_LITERAL_PATHSPECS")
	os.Setenv("GIT_LITERAL_PATHSPECS", "1")
	defer func() {
		if had {
			os.Setenv("GIT_LITERAL_PATHSPECS", old)
		} else {
			os.Unsetenv("GIT_LITERAL_PATHSPECS")
		}
	}()
	return fn()
}

// doPaths is do for commands given file paths (see literalPaths).
func (r *Repo) doPaths(fn func(g *commands.GitCommand) error) error {
	return r.do(func(g *commands.GitCommand) error { return literalPaths(func() error { return fn(g) }) })
}

// withPaths is with for commands given file paths (see literalPaths).
func withPaths[T any](r *Repo, fn func() (T, error)) (T, error) {
	return with(r, func() (T, error) {
		var v T
		err := literalPaths(func() error {
			var err error
			v, err = fn()
			return err
		})
		return v, err
	})
}

// ErrClosed is the error of a command of a repository closed.
var ErrClosed = errors.New("the repository is closed")

// enter waits for the commands of other repositories to finish, and makes
// r current: the working directory its work tree, and git's environment
// its own. It returns the function that leaves. When the work tree is gone,
// as when it was moved, it fails, leaving nothing current: a command must
// never run in another repository's directory.
func (r *Repo) enter() (leave func(), err error) {
	if r.closing.Load() {
		return nil, ErrClosed
	}
	gitMu.Lock()
	if r.closed || r.closing.Load() {
		gitMu.Unlock()
		return nil, ErrClosed
	}
	if err := os.Chdir(r.Path); err != nil {
		current = nil
		env.UnsetGitLocationEnvVars()
		gitMu.Unlock()
		return nil, fmt.Errorf("the repository's folder is gone: %w", err)
	}
	if current != r {
		env.SetGitLocationEnvVars(r.git.RepoPaths.GitLocationEnvVars())
		current = r
	}
	return gitMu.Unlock, nil
}

// Open opens the repository dir is in. lazygit's constructor changes the
// working directory and git's environment, so it takes its turn as
// commands do. It blocks: call it off the UI thread.
func Open(dir string, opts Options) (r *Repo, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	gitMu.Lock()
	defer gitMu.Unlock()
	// The repository finds itself from the directory alone, not from the
	// git environment of the one current.
	current = nil
	env.UnsetGitLocationEnvVars()
	if err := os.Chdir(abs); err != nil {
		return nil, err
	}
	if opts.ConfigDir != "" {
		if err := os.MkdirAll(opts.ConfigDir, 0o700); err != nil {
			return nil, err
		}
		os.Setenv("CONFIG_DIR", opts.ConfigDir)
	}
	tempDir, err := os.MkdirTemp("", "laziergit-*")
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			os.RemoveAll(tempDir)
		}
	}()
	appConfig, err := config.NewAppConfig("laziergit", "poc", "", "", "", false, tempDir)
	if err != nil {
		return nil, err
	}

	logger := logrus.New()
	logger.SetOutput(io.Discard)
	cmn := &common.Common{
		Log:      logrus.NewEntry(logger),
		Tr:       i18n.EnglishTranslationSet(),
		AppState: appConfig.GetAppState(),
		Fs:       afero.NewOsFs(),
	}
	cmn.SetUserConfig(appConfig.GetUserConfig())

	onCommand := opts.OnCommand
	if onCommand == nil {
		onCommand = func(string, bool) {}
	}
	ask := opts.AskCredential
	if ask == nil {
		// A nil channel tells lazygit to stop the command.
		ask = func(Credential) <-chan string { return nil }
	}
	guiIO := oscommands.NewGuiIO(cmn.Log, onCommand, func() io.Writer { return io.Discard }, ask)
	osCmd := oscommands.NewOSCommand(cmn, appConfig, oscommands.GetPlatform(), guiIO)

	version, err := git_commands.GetGitVersion(osCmd)
	if err != nil {
		return nil, fmt.Errorf("reading git's version: %w", err)
	}
	gitConfig := git_config.NewStdCachedGitConfig(cmn.Log)
	diffRenderers := config.NewDiffRendererConfigManager(cmn.UserConfig)
	git, err := commands.NewGitCommand(cmn, version, osCmd, gitConfig, diffRenderers)
	if err != nil {
		return nil, err
	}
	r = &Repo{
		cmn:          cmn,
		osCmd:        osCmd,
		git:          git,
		mainBranches: git_commands.NewMainBranches(cmn, osCmd.Cmd),
		hashPool:     &utils.StringPool{},
		tempDir:      tempDir,
		Path:         git.RepoPaths.WorktreePath(),
	}
	current = r
	return r, nil
}

// Closing marks the repository as about to close, at once: commands that
// haven't started fail from then on. Close closes it.
func (r *Repo) Closing() { r.closing.Store(true) }

// Close closes the repository: its commands fail from then on, and its
// temporary files are removed. It waits for a command running: call it off
// the UI thread.
func (r *Repo) Close() {
	r.closing.Store(true)
	gitMu.Lock()
	r.closed = true
	if current == r {
		current = nil
		env.UnsetGitLocationEnvVars()
	}
	gitMu.Unlock()
	os.RemoveAll(r.tempDir)
}

// Name is the work tree's directory name.
func (r *Repo) Name() string { return filepath.Base(r.Path) }

// CommitLimit is how many commits a snapshot loads.
const CommitLimit = 300

// Snapshot is the state of a repository at one time. It is not changed
// after Load returns it.
type Snapshot struct {
	Files    []*models.File
	Branches []*models.Branch
	Commits  []*models.Commit
	Stash    []*models.StashEntry
	Remotes  []*models.Remote
	// Graph is the commit graph's rows, one per commit.
	Graph [][]Pipe
	// Head is the branch checked out, nil while the head is detached and
	// there are no branches.
	Head *models.Branch
	// State is the operation in progress, as a rebase.
	State models.WorkingTreeState
}

// Load reads the repository's state. background marks a refresh the user
// didn't ask for, which takes no optional locks.
func (r *Repo) Load(background bool) (*Snapshot, error) {
	return with(r, func() (*Snapshot, error) { return r.load(background) })
}

// with runs fn with r current (see enter).
func with[T any](r *Repo, fn func() (T, error)) (T, error) {
	leave, err := r.enter()
	if err != nil {
		var zero T
		return zero, err
	}
	defer leave()
	return fn()
}

func (r *Repo) load(background bool) (*Snapshot, error) {
	l := r.git.Loaders
	s := &Snapshot{}
	s.Files = l.FileLoader.GetStatusFiles(git_commands.GetStatusFileOptions{Background: background})
	branches, err := l.BranchLoader.Load(nil, r.mainBranches, nil, false, func(f func() error) { _ = f() }, func() {})
	if err != nil {
		return nil, fmt.Errorf("loading branches: %w", err)
	}
	s.Branches = branches
	opts := git_commands.GetCommitsOptions{
		Limit:                CommitLimit,
		IncludeRebaseCommits: true,
		RefName:              "HEAD",
		MainBranches:         r.mainBranches,
		HashPool:             r.hashPool,
	}
	for _, b := range branches {
		if b.Head {
			s.Head = b
			opts.RefForPushedStatus = b
		}
	}
	commits, err := l.CommitLoader.GetCommits(opts)
	if err != nil {
		return nil, fmt.Errorf("loading commits: %w", err)
	}
	s.Commits = commits
	s.Graph = GraphRows(commits)
	s.Stash = l.StashLoader.GetStashEntries("")
	s.Remotes = l.RemoteLoader.GetRemotes()
	s.State = r.git.Status.WorkingTreeState()
	return s, nil
}

// FileDiff is the diff of a file of the work tree: its staged changes, or
// those not staged yet.
func (r *Repo) FileDiff(f *models.File, staged bool) (string, error) {
	return withPaths(r, func() (string, error) {
		return r.git.WorkingTree.WorktreeFileDiff(f, git_commands.DiffModePlain, staged), nil
	})
}

// CommitFiles lists the files a commit changes, against its first parent.
func (r *Repo) CommitFiles(c *models.Commit) ([]*models.CommitFile, error) {
	return with(r, func() ([]*models.CommitFile, error) {
		return r.git.Loaders.CommitFileLoader.GetFilesInDiff(c.ParentRefName(), c.RefName(), false)
	})
}

// CommitFileDiff is the diff of a file a commit changes.
func (r *Repo) CommitFileDiff(c *models.Commit, f *models.CommitFile) (string, error) {
	return withPaths(r, func() (string, error) {
		return r.git.WorkingTree.ShowFileDiff(c.ParentRefName(), c.RefName(), false, f.Path, f.PreviousPath, git_commands.DiffModePlain)
	})
}

// StashFiles lists the files a stash entry changes.
func (r *Repo) StashFiles(e *models.StashEntry) ([]*models.CommitFile, error) {
	return with(r, func() ([]*models.CommitFile, error) {
		return r.git.Loaders.CommitFileLoader.GetFilesInDiff(e.ParentRefName(), e.RefName(), false)
	})
}

// StashFileDiff is the diff of a file a stash entry changes.
func (r *Repo) StashFileDiff(e *models.StashEntry, f *models.CommitFile) (string, error) {
	return withPaths(r, func() (string, error) {
		return r.git.WorkingTree.ShowFileDiff(e.ParentRefName(), e.RefName(), false, f.Path, f.PreviousPath, git_commands.DiffModePlain)
	})
}

func (r *Repo) do(fn func(g *commands.GitCommand) error) error {
	leave, err := r.enter()
	if err != nil {
		return err
	}
	defer leave()
	return cleanError(fn(r.git))
}

// cleanError trims the output git wrapped into an error, keeping the
// error it wraps for errors.Is.
func cleanError(err error) error {
	if err == nil {
		return nil
	}
	return trimmedError{err}
}

type trimmedError struct{ err error }

func (e trimmedError) Error() string { return strings.TrimSpace(e.err.Error()) }
func (e trimmedError) Unwrap() error { return e.err }

// Tr is lazygit's English strings, as for a working tree state's title.
func (r *Repo) Tr() *i18n.TranslationSet { return r.cmn.Tr }
