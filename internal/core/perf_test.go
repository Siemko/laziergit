package core

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// TestLoadTimes tells how long each step of a load takes in the
// repository $LAZIERGIT_PERF_REPO, the best of a few loads, for finding
// what makes a large repository slow:
//
//	LAZIERGIT_PERF_REPO=~/src/go go test -run TestLoadTimes -v ./internal/core
func TestLoadTimes(t *testing.T) {
	dir := os.Getenv("LAZIERGIT_PERF_REPO")
	if dir == "" {
		t.Skip("LAZIERGIT_PERF_REPO not set")
	}
	cwd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(cwd) })
	r, err := Open(dir, Options{ConfigDir: filepath.Join(t.TempDir(), "config")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	best := map[string]time.Duration{}
	r.trace = func(step string, d time.Duration) {
		if b, ok := best[step]; !ok || d < b {
			best[step] = d
		}
	}
	var total time.Duration
	for i := range 4 {
		start := time.Now()
		s, err := r.Load(true)
		if err != nil {
			t.Fatal(err)
		}
		d := time.Since(start)
		if i == 0 || d < total {
			total = d
		}
		if i == 0 {
			t.Logf("%d files, %d branches, %d commits, %d tags, %d reflog entries, %d worktrees, %d submodules",
				len(s.Files), len(s.Branches), len(s.Commits), len(s.Tags), len(s.Reflog), len(s.Worktrees), len(s.Submodules))
		}
	}
	steps := make([]string, 0, len(best))
	for step := range best {
		steps = append(steps, step)
	}
	sort.Slice(steps, func(i, j int) bool { return best[steps[i]] > best[steps[j]] })
	for _, step := range steps {
		t.Logf("%-16s %8.1f ms", step, float64(best[step].Microseconds())/1000)
	}
	t.Logf("%-16s %8.1f ms", "load", float64(total.Microseconds())/1000)
}
