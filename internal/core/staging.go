package core

import (
	"errors"
	"strings"

	"github.com/jesseduffield/lazygit/pkg/commands"
	"github.com/jesseduffield/lazygit/pkg/commands/git_commands"
	"github.com/jesseduffield/lazygit/pkg/commands/models"
	"github.com/jesseduffield/lazygit/pkg/commands/patch"
)

// LineAction is what ApplyLines does with the lines chosen of a diff.
type LineAction int

const (
	// StageLines puts lines of the unstaged diff into the index.
	StageLines LineAction = iota
	// UnstageLines takes lines of the staged diff out of the index.
	UnstageLines
	// DiscardLines throws lines of the unstaged diff away from the work tree.
	DiscardLines
)

// ErrStale is the error of acting on a diff that is no longer the file's.
var ErrStale = errors.New("the file changed since its diff was shown: look at it again")

// ApplyLines acts on some of the lines of a file's diff, as lazygit's
// staging does: diff is the text the lines were chosen in (FileDiff's), and
// indices are into patch.Parse(diff).Lines(). Lines that are not changes
// are ignored. Choosing every change acts on the whole file, as staging or
// unstaging it, so that an added or deleted file is added or deleted
// rather than emptied.
//
// It acts only on the diff shown: when the file's diff is no longer diff,
// as after an edit since, it fails with ErrStale rather than act on lines
// the user hasn't seen.
func (r *Repo) ApplyLines(f *models.File, diff string, indices []int, action LineAction) error {
	if f.HasInlineMergeConflicts {
		return errConflicted(f.Path)
	}
	p := patch.Parse(diff)
	lines := p.Lines()
	var chosen []int
	for _, i := range indices {
		if i >= 0 && i < len(lines) && lines[i].IsChange() {
			chosen = append(chosen, i)
		}
	}
	if len(chosen) == 0 {
		return errors.New("no changed lines chosen")
	}
	changes := 0
	for _, l := range lines {
		if l.IsChange() {
			changes++
		}
	}
	whole := len(chosen) == changes
	if !whole && !patchablePath(f.Path) {
		// A patch's header names the file raw: a tab or a newline in the
		// name would make it another file's.
		return errors.New("lines of " + f.Path + " can't be chosen, as its name holds a tab, a newline, a quote or a backslash: act on the whole file")
	}
	reverse := action != StageLines
	opts := git_commands.ApplyPatchOpts{Reverse: reverse, Cached: action != DiscardLines}
	text := p.Transform(patch.TransformOpts{
		Reverse:             reverse,
		IncludedLineIndices: chosen,
		FileNameOverride:    f.Path,
	}).FormatPlain()

	return r.do(func(g *commands.GitCommand) error {
		if live := g.WorkingTree.WorktreeFileDiff(f, git_commands.DiffModePlain, action == UnstageLines); live != diff {
			return ErrStale
		}
		switch {
		case whole && action == StageLines:
			return g.WorkingTree.StageFile(f.Path)
		case whole && action == UnstageLines:
			return g.WorkingTree.UnStageFile(f.Names(), f.Tracked)
		case whole && action == DiscardLines && f.ShortStatus == "??":
			// Untracked: discarding all of it is removing it. A file
			// added to the index and changed since ("AM") keeps what is
			// staged, as the reverse patch below does.
			return g.WorkingTree.DiscardAllFileChanges(f)
		}
		return g.Patch.ApplyPatch(text, opts)
	})
}

// patchablePath reports whether a path can stand raw in a patch's header.
func patchablePath(path string) bool {
	return !strings.ContainsAny(path, "\t\n\r\"\\")
}

func errConflicted(path string) error {
	return errors.New(path + " has merge conflicts: resolve them first, or its conflict markers would be staged")
}
