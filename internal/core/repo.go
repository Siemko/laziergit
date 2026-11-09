// Package core drives a git repository through lazygit's command layer
// (github.com/jesseduffield/lazygit/pkg/commands), for a UI to show and act on.
package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// literal is a path as a pathspec git reads as the file's name, never a
// pattern: "app/[slug]/page.tsx" or ":(glob)*" stands for itself, not for
// the files it would match. The magic is in each argument rather than in
// GIT_LITERAL_PATHSPECS, which git would hand on to hooks, filters and diff
// drivers, whose own patterns would stop matching.
func literal(path string) string { return ":(literal)" + path }

func literals(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = literal(p)
	}
	return out
}

// gitRun runs git in the current repository (see enter), as lazygit's
// commands run, logged to the command log.
func (r *Repo) gitRun(args ...string) error {
	return r.osCmd.Cmd.New(append([]string{"git"}, args...)).Run()
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
	return with(r, func() (string, error) { return r.fileDiff(f, staged) })
}

// fileDiff is FileDiff with the repository current. Its paths are literal
// pathspecs, but for a file not in the index, which git diff --no-index
// takes as a path of the file system. Its only error is ErrTooLarge.
func (r *Repo) fileDiff(f *models.File, staged bool) (string, error) {
	noIndex := !f.GetIsTracked() && !f.GetHasStagedChanges() && !staged && f.GetIsFile()
	paths := f.Names()
	if !noIndex {
		paths = literals(paths)
	}
	s, err := boundedOutput(r.git.WorkingTree.WorktreeFileDiffCmdObj(f, git_commands.DiffModePlain, staged, paths))
	if errors.Is(err, ErrTooLarge) {
		return "", err
	}
	// Another error is a file deleted, as lazygit takes it.
	return s, nil
}

// showFileDiff is the diff of a file between two commits.
func (r *Repo) showFileDiff(from, to string, f *models.CommitFile) (string, error) {
	paths := []string{literal(f.Path)}
	if f.PreviousPath != "" {
		paths = append(paths, literal(f.PreviousPath))
	}
	return boundedOutput(r.git.WorkingTree.ShowFileDiffCmdObj(from, to, false, paths, git_commands.DiffModePlain))
}

// maxDiffSize is the largest diff read: a larger one would take memory,
// and the UI thread's time to parse and highlight it, the app can't spare.
const maxDiffSize = 8 << 20

// ErrTooLarge is the error of a diff larger than maxDiffSize.
var ErrTooLarge = errors.New("the diff is too large to show here")

// maxErrorOutput is how much of a command's standard error boundedOutput
// keeps, for its error.
const maxErrorOutput = 64 << 10

// boundedOutput runs a command for its output, as RunWithOutput does but
// for its standard error, which it leaves out of the output, and failing
// with ErrTooLarge as soon as the output passes maxDiffSize: it is never
// read whole. Of the standard error, the start is kept, for the error.
// What the command started and left holding its output, as a filter of
// git's, isn't waited on long: the repository is every window's.
func boundedOutput(obj *oscommands.CmdObj) (string, error) {
	cmd := obj.GetCmd()
	out := &cappedWriter{max: maxDiffSize}
	stderr := &cappedWriter{max: maxErrorOutput, drop: true}
	cmd.Stdout, cmd.Stderr = out, stderr
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	switch {
	case out.over:
		return "", ErrTooLarge
	case errors.Is(err, exec.ErrWaitDelay):
		// git finished; what it left running held on to its output.
	case err != nil:
		return out.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

// cappedWriter keeps what is written up to max bytes. Past that, it fails
// the write, which stops the copy from the command's pipe and closes it:
// the command, writing on, stops on a broken pipe. With drop, it takes
// what is past max and leaves it out instead.
type cappedWriter struct {
	strings.Builder
	max  int
	drop bool
	over bool
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	if room := w.max - w.Len(); len(p) > room {
		if !w.drop {
			w.over = true
			return 0, ErrTooLarge
		}
		w.Builder.Write(p[:max(room, 0)])
		return len(p), nil
	}
	return w.Builder.Write(p)
}

// CommitFiles lists the files a commit changes, against its first parent.
func (r *Repo) CommitFiles(c *models.Commit) ([]*models.CommitFile, error) {
	return with(r, func() ([]*models.CommitFile, error) {
		return r.git.Loaders.CommitFileLoader.GetFilesInDiff(c.ParentRefName(), c.RefName(), false)
	})
}

// CommitFileDiff is the diff of a file a commit changes.
func (r *Repo) CommitFileDiff(c *models.Commit, f *models.CommitFile) (string, error) {
	return with(r, func() (string, error) { return r.showFileDiff(c.ParentRefName(), c.RefName(), f) })
}

// StashFiles lists the files a stash entry changes.
func (r *Repo) StashFiles(e *models.StashEntry) ([]*models.CommitFile, error) {
	return with(r, func() ([]*models.CommitFile, error) {
		return r.git.Loaders.CommitFileLoader.GetFilesInDiff(e.ParentRefName(), e.RefName(), false)
	})
}

// StashFileDiff is the diff of a file a stash entry changes.
func (r *Repo) StashFileDiff(e *models.StashEntry, f *models.CommitFile) (string, error) {
	return with(r, func() (string, error) { return r.showFileDiff(e.ParentRefName(), e.RefName(), f) })
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
