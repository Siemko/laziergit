package core

import (
	"slices"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands"
)

// Config is what of the app's settings git's commands follow, which a
// repository takes in place of lazygit's config.yml.
type Config struct {
	// DiffContext is the lines of context around a diff's changes, 1 or
	// more: line staging needs some.
	DiffContext int
	// CommitLimit is how many commits a snapshot loads.
	CommitLimit int
	// SignOff adds a Signed-off-by trailer to the commits made.
	SignOff bool
	// MainBranches are the branches whose commits count as merged, as
	// master and main.
	MainBranches []string
}

// DefaultConfig is lazygit's defaults, and the app's limit.
func DefaultConfig() Config {
	return Config{DiffContext: 3, CommitLimit: CommitLimit, MainBranches: []string{"master", "main"}}
}

// Normalized holds a config to what the app can use.
func (c Config) Normalized() Config {
	def := DefaultConfig()
	if c.DiffContext < 1 {
		c.DiffContext = def.DiffContext
	}
	c.DiffContext = min(c.DiffContext, 100)
	if c.CommitLimit < 1 {
		c.CommitLimit = def.CommitLimit
	}
	c.CommitLimit = min(c.CommitLimit, 100_000)
	var mains []string
	for _, b := range c.MainBranches {
		// A name git would read as an option is no branch of git's.
		if b = strings.TrimSpace(b); b != "" && !strings.HasPrefix(b, "-") && !slices.Contains(mains, b) {
			mains = append(mains, b)
		}
	}
	c.MainBranches = mains
	return c
}

// Configure makes the repository's commands follow c, from the next
// command on. It waits for a command running: call it off the UI thread.
func (r *Repo) Configure(c Config) error {
	c = c.Normalized()
	leave, err := r.enter()
	if err != nil {
		return err
	}
	defer leave()
	// Commands read the config as they run, under the lock: a copy with
	// the settings takes its place.
	uc := *r.cmn.UserConfig()
	uc.Git.DiffContextSize = uint64(c.DiffContext)
	uc.Git.Commit.SignOff = c.SignOff
	uc.Git.MainBranches = c.MainBranches
	r.cmn.SetUserConfig(&uc)
	r.commitLimit.Store(int64(c.CommitLimit))
	return nil
}

// Limit is how many commits a snapshot loads.
func (r *Repo) Limit() int {
	if n := r.commitLimit.Load(); n > 0 {
		return int(n)
	}
	return CommitLimit
}

// FetchBackground fetches, as lazygit does on its own every few minutes:
// without the command log, and failing rather than asking for a password.
func (r *Repo) FetchBackground() error {
	return r.do(func(g *commands.GitCommand) error { return g.Sync.FetchBackground() })
}
