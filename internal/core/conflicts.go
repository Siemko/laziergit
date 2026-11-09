package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

// Pick is the side of a conflict kept.
type Pick int

const (
	PickOurs   Pick = iota // the lines of HEAD, "current"
	PickTheirs             // the lines merged in, "incoming"
	PickBoth               // ours, then theirs
	PickBase               // the common ancestor's, with the diff3 style
)

// Conflict is a conflict of a file: the indices of its marker lines, as
// lazygit finds them, Base -1 without the diff3 style.
type Conflict struct {
	Start, Base, Middle, End int
	// OursLabel and TheirsLabel are what the markers say, as "HEAD" and
	// "feature".
	OursLabel, TheirsLabel string
}

// Ours, BaseLines and Theirs are the ranges of the conflict's sides, as
// [from, to) indices into the file's lines.
func (c Conflict) Ours() (int, int) {
	if c.Base >= 0 {
		return c.Start + 1, c.Base
	}
	return c.Start + 1, c.Middle
}

func (c Conflict) BaseLines() (int, int) {
	if c.Base < 0 {
		return 0, 0
	}
	return c.Base + 1, c.Middle
}

func (c Conflict) Theirs() (int, int) { return c.Middle + 1, c.End }

// ConflictedFile is a file with conflict markers, split into lines that
// keep their line endings.
type ConflictedFile struct {
	Lines     []string
	Conflicts []Conflict
}

// ParseConflicts finds the conflicts of a file's content, with lazygit's
// rules: markers of markerSize characters (7 when 0, the
// conflict-marker-size attribute otherwise), the start, base and end ones
// with an optional label after a space, the middle one alone.
func ParseConflicts(content string, markerSize int) *ConflictedFile {
	if markerSize < 1 {
		markerSize = 7
	}
	f := &ConflictedFile{Lines: strings.SplitAfter(content, "\n")}
	if n := len(f.Lines); n > 0 && f.Lines[n-1] == "" {
		f.Lines = f.Lines[:n-1]
	}
	marker := func(line string, ch byte) (label string, ok bool) {
		line = strings.TrimRight(line, "\r\n")
		if len(line) < markerSize || strings.Count(line[:markerSize], string(ch)) != markerSize {
			return "", false
		}
		rest := line[markerSize:]
		if ch == '=' {
			return "", rest == ""
		}
		if rest == "" {
			return "", true
		}
		if rest[0] != ' ' {
			return "", false
		}
		return rest[1:], true
	}
	var cur *Conflict
	for i, line := range f.Lines {
		if label, ok := marker(line, '<'); ok {
			cur = &Conflict{Start: i, Base: -1, Middle: -1, OursLabel: label}
		} else if cur == nil {
			continue
		} else if _, ok := marker(line, '|'); ok && cur.Middle < 0 {
			cur.Base = i
		} else if _, ok := marker(line, '='); ok && cur.Middle < 0 {
			cur.Middle = i
		} else if label, ok := marker(line, '>'); ok && cur.Middle >= 0 {
			cur.End, cur.TheirsLabel = i, label
			f.Conflicts = append(f.Conflicts, *cur)
			cur = nil
		}
	}
	return f
}

// Resolve is the file's content with conflict i resolved by pick, the
// other conflicts left as they are.
func (f *ConflictedFile) Resolve(i int, pick Pick) string {
	return f.resolve(func(j int) (Pick, bool) { return pick, j == i })
}

// ResolveAll is the file's content with every conflict resolved by pick.
func (f *ConflictedFile) ResolveAll(pick Pick) string {
	return f.resolve(func(int) (Pick, bool) { return pick, true })
}

// resolve keeps, of each conflict pickOf resolves, the lines of its pick,
// and every other line as it is.
func (f *ConflictedFile) resolve(pickOf func(i int) (Pick, bool)) string {
	var b strings.Builder
	write := func(from, to int) {
		for _, l := range f.Lines[from:to] {
			b.WriteString(l)
		}
	}
	at := 0
	for i, c := range f.Conflicts {
		pick, ok := pickOf(i)
		if !ok {
			continue
		}
		write(at, c.Start)
		of, ot := c.Ours()
		tf, tt := c.Theirs()
		switch pick {
		case PickOurs:
			write(of, ot)
		case PickTheirs:
			write(tf, tt)
		case PickBoth:
			write(of, ot)
			write(tf, tt)
		case PickBase:
			write(c.BaseLines())
		}
		at = c.End + 1
	}
	write(at, len(f.Lines))
	return b.String()
}

// ErrNotRegular is the error of reading or writing, as text, a conflicted
// path that isn't a regular file of the work tree, as a symlink: a
// repository's symlink could point anywhere.
var ErrNotRegular = errors.New("not a regular file of the work tree")

// maxConflictedSize is the largest file the conflict view reads.
const maxConflictedSize = 32 << 20

// workTreeFile is the full path of a file of the work tree, given by its
// path relative to it, as git status gives it: a regular file, not a
// symlink, in no directory a symlink leads out of the work tree to.
func (r *Repo) workTreeFile(path string) (string, os.FileInfo, error) {
	if !filepath.IsLocal(path) {
		return "", nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	full := filepath.Join(r.Path, path)
	root, err := filepath.EvalSymlinks(r.Path)
	if err != nil {
		return "", nil, err
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return "", nil, err
	}
	if rel, err := filepath.Rel(root, dir); err != nil || !filepath.IsLocal(rel) && rel != "." {
		return "", nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	st, err := os.Lstat(full)
	if err != nil {
		return "", nil, err
	}
	if !st.Mode().IsRegular() {
		return "", nil, fmt.Errorf("%s: %w", path, ErrNotRegular)
	}
	return full, st, nil
}

// ReadConflicted reads a conflicted file of the work tree, failing with
// ErrNotRegular for one that isn't a regular file.
func (r *Repo) ReadConflicted(path string) (string, error) {
	full, st, err := r.workTreeFile(path)
	if err != nil {
		return "", err
	}
	if st.Size() > maxConflictedSize {
		return "", fmt.Errorf("%s is too large to resolve here", path)
	}
	b, err := os.ReadFile(full)
	return string(b), err
}

// ResolveConflict writes content, a resolution of the conflicted file f,
// in place of expected, the content it was resolved from. It fails with
// ErrStale when the file is no longer expected, as after an edit in
// another app since, rather than overwrite that. With no conflict left in
// content, the file is staged, as resolved; staged reports it.
func (r *Repo) ResolveConflict(f *models.File, expected, content string) (staged bool, err error) {
	err = r.do(func(g *commands.GitCommand) error {
		full, st, err := r.workTreeFile(f.Path)
		if err != nil {
			return err
		}
		now, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if string(now) != expected {
			return ErrStale
		}
		if err := writeAtomically(full, content, st.Mode().Perm()); err != nil {
			return err
		}
		if len(ParseConflicts(content, f.ConflictMarkerSize).Conflicts) > 0 {
			return nil
		}
		staged = true
		return g.WorkingTree.StageFile(f.Path)
	})
	return staged, err
}

// writeAtomically replaces a file with content: a reader sees the old file
// or the new one, never a part.
func writeAtomically(path, content string, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".laziergit-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// MarkResolved stages a file whose conflicts are resolved: one without
// conflict markers, or a conflict git wrote no markers for.
func (r *Repo) MarkResolved(f *models.File) error {
	if f.HasInlineMergeConflicts {
		if content, err := r.ReadConflicted(f.Path); err == nil && len(ParseConflicts(content, f.ConflictMarkerSize).Conflicts) > 0 {
			return errConflicted(f.Path)
		}
	}
	return r.do(func(g *commands.GitCommand) error { return g.WorkingTree.StageFile(f.Path) })
}

// ContinueOperation continues the merge, rebase, cherry-pick or revert in
// progress, skipping the editor for a commit's message; SkipOperation
// skips its current commit, and AbortOperation gives it up.
func (r *Repo) ContinueOperation(st models.WorkingTreeState) error {
	return r.operation(st, "continue")
}

func (r *Repo) SkipOperation(st models.WorkingTreeState) error { return r.operation(st, "skip") }

func (r *Repo) AbortOperation(st models.WorkingTreeState) error { return r.operation(st, "abort") }

func (r *Repo) operation(st models.WorkingTreeState, command string) error {
	if st.None() {
		return errors.New("no merge or rebase is in progress")
	}
	return r.do(func(g *commands.GitCommand) error {
		return g.Rebase.GenericMergeOrRebaseAction(st.Effective(), command)
	})
}

// IsConflict reports whether err is git stopping on conflicts, as a merge,
// rebase, pull or cherry-pick does.
func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "CONFLICT") || strings.Contains(s, "conflict") && strings.Contains(s, "fix")
}
