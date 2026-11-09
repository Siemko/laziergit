// Package core drives a git repository through lazygit's command layer
// (github.com/jesseduffield/lazygit/pkg/commands), for a UI to show and act on.
package core

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// enter waits for the commands of other repositories to finish, and makes
// r current: the working directory its work tree, and git's environment
// its own. It returns the function that leaves.
func (r *Repo) enter() (leave func()) {
	gitMu.Lock()
	if current != r {
		if err := os.Chdir(r.Path); err != nil {
			r.cmn.Log.Error(err)
		}
		env.SetGitLocationEnvVars(r.git.RepoPaths.GitLocationEnvVars())
		current = r
	}
	return gitMu.Unlock
}

// Open opens the repository dir is in. lazygit's constructor changes the
// working directory and git's environment, so it takes its turn as
// commands do.
func Open(dir string, opts Options) (*Repo, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	gitMu.Lock()
	defer gitMu.Unlock()
	// A failed Open leaves the directory as it was for the repository
	// current; a new one is current when it opens.
	defer func() {
		if current != nil {
			_ = os.Chdir(current.Path)
			env.SetGitLocationEnvVars(current.git.RepoPaths.GitLocationEnvVars())
		}
	}()
	if err := os.Chdir(abs); err != nil {
		return nil, err
	}
	if opts.ConfigDir != "" {
		if err := os.MkdirAll(opts.ConfigDir, 0o755); err != nil {
			return nil, err
		}
		os.Setenv("CONFIG_DIR", opts.ConfigDir)
	}
	tempDir, err := os.MkdirTemp("", "laziergit-*")
	if err != nil {
		return nil, err
	}
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
	r := &Repo{
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

// Close removes the repository's temporary files.
func (r *Repo) Close() {
	gitMu.Lock()
	if current == r {
		current = nil
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
	defer r.enter()()
	return r.load(background)
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
func (r *Repo) FileDiff(f *models.File, staged bool) string {
	defer r.enter()()
	return r.git.WorkingTree.WorktreeFileDiff(f, git_commands.DiffModePlain, staged)
}

// CommitFiles lists the files a commit changes, against its first parent.
func (r *Repo) CommitFiles(c *models.Commit) ([]*models.CommitFile, error) {
	defer r.enter()()
	return r.git.Loaders.CommitFileLoader.GetFilesInDiff(c.ParentRefName(), c.RefName(), false)
}

// CommitFileDiff is the diff of a file a commit changes.
func (r *Repo) CommitFileDiff(c *models.Commit, f *models.CommitFile) (string, error) {
	defer r.enter()()
	return r.git.WorkingTree.ShowFileDiff(c.ParentRefName(), c.RefName(), false, f.Path, f.PreviousPath, git_commands.DiffModePlain)
}

// StashFiles lists the files a stash entry changes.
func (r *Repo) StashFiles(e *models.StashEntry) ([]*models.CommitFile, error) {
	defer r.enter()()
	return r.git.Loaders.CommitFileLoader.GetFilesInDiff(e.ParentRefName(), e.RefName(), false)
}

// StashFileDiff is the diff of a file a stash entry changes.
func (r *Repo) StashFileDiff(e *models.StashEntry, f *models.CommitFile) (string, error) {
	defer r.enter()()
	return r.git.WorkingTree.ShowFileDiff(e.ParentRefName(), e.RefName(), false, f.Path, f.PreviousPath, git_commands.DiffModePlain)
}

func (r *Repo) do(fn func(g *commands.GitCommand) error) error {
	defer r.enter()()
	return cleanError(fn(r.git))
}

// cleanError trims the output git wrapped into an error.
func cleanError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", strings.TrimSpace(err.Error()))
}

// Tr is lazygit's English strings, as for a working tree state's title.
func (r *Repo) Tr() *i18n.TranslationSet { return r.cmn.Tr }
