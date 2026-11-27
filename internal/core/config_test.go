package core

import (
	"slices"
	"strings"
	"testing"
)

func TestConfigure(t *testing.T) {
	r, dir := newRepo(t)
	// A file of 20 lines, its 10th changed.
	var lines []string
	for i := range 20 {
		lines = append(lines, string(rune('a'+i)))
	}
	write(t, dir, "letters.txt", strings.Join(lines, "\n")+"\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "letters")
	lines[9] = "changed"
	write(t, dir, "letters.txt", strings.Join(lines, "\n")+"\n")

	diffLines := func() int {
		t.Helper()
		s := load(t, r)
		d, err := r.FileDiff(s.Files[0], false)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, l := range strings.Split(d, "\n") {
			if strings.HasPrefix(l, " ") {
				n++
			}
		}
		return n
	}
	if n := diffLines(); n != 6 {
		t.Fatalf("%d lines of context by default, want 6", n)
	}
	if err := r.Configure(Config{DiffContext: 1, CommitLimit: 2, SignOff: true, MainBranches: []string{"main", "--all", " main "}}); err != nil {
		t.Fatal(err)
	}
	if n := diffLines(); n != 2 {
		t.Fatalf("%d lines of context, want 2", n)
	}
	if s := load(t, r); len(s.Commits) != 2 || r.Limit() != 2 {
		t.Fatalf("%d commits loaded, limit %d", len(s.Commits), r.Limit())
	}
	if got := r.cmn.UserConfig().Git.MainBranches; !slices.Equal(got, []string{"main"}) {
		t.Fatalf("main branches %q", got)
	}
	git(t, dir, "add", ".")
	if err := r.Commit("Change j", "", false); err != nil {
		t.Fatal(err)
	}
	if body := git(t, dir, "log", "-1", "--format=%b"); !strings.HasPrefix(body, "Signed-off-by: Test <test@example.com>") {
		t.Fatalf("no sign-off in %q", body)
	}
	// Nothing set is the defaults.
	if c := (Config{}).Normalized(); c.DiffContext != 3 || c.CommitLimit != CommitLimit {
		t.Fatalf("normalized %+v", c)
	}
}
