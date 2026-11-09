package core

import (
	"errors"

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

// ApplyLines acts on some of the lines of a file's diff, as lazygit's
// staging does: diff is the text the lines were chosen in (FileDiff's), and
// indices are into patch.Parse(diff).Lines(). Lines that are not changes
// are ignored. Choosing every change acts on the whole file, as staging or
// unstaging it, so that an added or deleted file is added or deleted
// rather than emptied.
//
// git checks the diff still applies: when the file changed since diff was
// read, ApplyLines fails rather than act on lines that moved.
func (r *Repo) ApplyLines(f *models.File, diff string, indices []int, action LineAction) error {
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
	reverse := action != StageLines
	opts := git_commands.ApplyPatchOpts{Reverse: reverse, Cached: action != DiscardLines}
	text := p.Transform(patch.TransformOpts{
		Reverse:             reverse,
		IncludedLineIndices: chosen,
		FileNameOverride:    f.Path,
	}).FormatPlain()

	return r.do(func(g *commands.GitCommand) error {
		switch {
		case whole && action == StageLines:
			return g.WorkingTree.StageFile(f.Path)
		case whole && action == UnstageLines:
			return g.WorkingTree.UnStageFile(f.Names(), f.Tracked)
		case whole && action == DiscardLines && !f.Tracked:
			return g.WorkingTree.DiscardAllFileChanges(f)
		}
		return g.Patch.ApplyPatch(text, opts)
	})
}
