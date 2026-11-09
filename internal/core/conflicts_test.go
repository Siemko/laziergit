package core

import (
	"strings"
	"testing"
)

const conflicted = `top
<<<<<<< HEAD
ours 1
ours 2
=======
theirs
>>>>>>> feature
middle
<<<<<<< HEAD
a
||||||| base
b
=======
c
>>>>>>>
end
`

func TestParseAndResolveConflicts(t *testing.T) {
	f := ParseConflicts(conflicted, 0)
	if len(f.Conflicts) != 2 {
		t.Fatalf("conflicts: %+v", f.Conflicts)
	}
	c := f.Conflicts[0]
	if c.OursLabel != "HEAD" || c.TheirsLabel != "feature" || c.Base != -1 {
		t.Fatalf("first: %+v", c)
	}
	if c2 := f.Conflicts[1]; c2.Base < 0 || c2.TheirsLabel != "" {
		t.Fatalf("second, diff3: %+v", c2)
	}
	cases := []struct {
		i    int
		pick Pick
		want string
	}{
		{0, PickOurs, "top\nours 1\nours 2\nmiddle\n"},
		{0, PickTheirs, "top\ntheirs\nmiddle\n"},
		{0, PickBoth, "top\nours 1\nours 2\ntheirs\nmiddle\n"},
		{1, PickBase, "middle\nb\nend\n"},
	}
	for _, tc := range cases {
		got := f.Resolve(tc.i, tc.pick)
		if !strings.Contains(got, tc.want) {
			t.Errorf("Resolve(%d, %d):\n%s\nwants %q", tc.i, tc.pick, got, tc.want)
		}
		if left := ParseConflicts(got, 0); len(left.Conflicts) != 1 {
			t.Errorf("Resolve(%d, %d) leaves %d conflicts", tc.i, tc.pick, len(left.Conflicts))
		}
	}
	if got := f.ResolveAll(PickTheirs); got != "top\ntheirs\nmiddle\nc\nend\n" {
		t.Fatalf("ResolveAll: %q", got)
	}
}

func TestParseConflictsCRLFAndMarkerSize(t *testing.T) {
	crlf := strings.ReplaceAll("x\n<<<<<<< HEAD\no\n=======\nt\n>>>>>>> b\ny\n", "\n", "\r\n")
	f := ParseConflicts(crlf, 0)
	if len(f.Conflicts) != 1 || f.Resolve(0, PickTheirs) != "x\r\nt\r\ny\r\n" {
		t.Fatalf("crlf: %+v %q", f.Conflicts, f.Resolve(0, PickTheirs))
	}
	// Seven-character markers aren't markers in a file whose size is 9,
	// and a separator with a label never is one.
	nine := "<<<<<<<<< HEAD\n<<<<<<< not\n=========\n======= x\n>>>>>>>>> b\n"
	f = ParseConflicts(nine, 9)
	if len(f.Conflicts) != 1 || f.Resolve(0, PickTheirs) != "======= x\n" {
		t.Fatalf("size 9: %+v %q", f.Conflicts, f.Resolve(0, PickTheirs))
	}
}

func TestResolveMergeConflict(t *testing.T) {
	r, dir := newRepo(t)
	write(t, dir, "one.txt", "one\nmain's\n")
	git(t, dir, "commit", "-q", "-am", "main changes one")
	git(t, dir, "checkout", "-q", "-b", "feature", "HEAD~1")
	write(t, dir, "one.txt", "one\nfeature's\n")
	git(t, dir, "commit", "-q", "-am", "feature changes one")
	git(t, dir, "checkout", "-q", "main")

	err := r.Merge("feature")
	if !IsConflict(err) {
		t.Fatalf("merge: %v", err)
	}
	s := load(t, r)
	if !s.State.Merging {
		t.Fatalf("state %+v", s.State)
	}
	f := fileNamed(t, s, "one.txt")
	if !f.HasInlineMergeConflicts {
		t.Fatalf("file %+v", f)
	}
	content, err := r.ReadFile(f.Path)
	if err != nil {
		t.Fatal(err)
	}
	cf := ParseConflicts(content, f.ConflictMarkerSize)
	if len(cf.Conflicts) != 1 || cf.Conflicts[0].TheirsLabel != "feature" {
		t.Fatalf("conflicts %+v", cf.Conflicts)
	}
	if err := r.WriteFile(f.Path, cf.Resolve(0, PickBoth)); err != nil {
		t.Fatal(err)
	}
	if err := r.MarkResolved(f); err != nil {
		t.Fatal(err)
	}
	if err := r.ContinueOperation(load(t, r).State); err != nil {
		t.Fatal(err)
	}
	if s := load(t, r); !s.State.None() || !s.Commits[0].IsMerge() {
		t.Fatalf("after continue: state %+v, head %q", s.State, s.Commits[0].Name)
	}
	if got := git(t, dir, "show", "HEAD:one.txt"); got != "one\nmain's\nfeature's" {
		t.Fatalf("merged one.txt: %q", got)
	}
}

func TestAbortRebaseConflict(t *testing.T) {
	r, dir := newRepo(t)
	git(t, dir, "checkout", "-q", "-b", "feature", "HEAD~1")
	write(t, dir, "three.txt", "feature's three\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feature adds three")
	if err := r.RebaseOnto("main"); !IsConflict(err) {
		t.Fatalf("rebase: %v", err)
	}
	if s := load(t, r); !s.State.Rebasing {
		t.Fatalf("state %+v", s.State)
	}
	if err := r.AbortOperation(load(t, r).State); err != nil {
		t.Fatal(err)
	}
	if s := load(t, r); !s.State.None() || s.Head.Name != "feature" {
		t.Fatalf("after abort: %+v on %v", s.State, s.Head.Name)
	}
}
