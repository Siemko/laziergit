package core

// The commit graph's layout is lazygit's (pkg/gui/presentation/graph,
// MIT, © 2018 Jesse Duffield), whose pipes are unexported: here they are
// kept as geometry for the UI to draw, without lazygit's terminal styles.

import (
	"cmp"
	"hash/fnv"
	"slices"

	"github.com/jesseduffield/lazygit/pkg/commands/models"
)

// PipeKind is how a pipe meets a row.
type PipeKind uint8

const (
	// Terminates: the pipe comes from above and ends at the row's commit.
	Terminates PipeKind = iota
	// Starts: the pipe leaves the row's commit for one below.
	Starts
	// Continues: the pipe passes the row, from its top to its bottom.
	Continues
)

// Pipe is a line of the graph through one row, between columns From (at
// the row's top, or its commit for Starts) and To (at its bottom, or its
// commit for Terminates).
type Pipe struct {
	From, To int16
	Kind     PipeKind
	// Color is the index of the pipe's color, from the commit it leaves.
	Color    uint8
	fromHash *string
	toHash   *string
}

var (
	emptyTreeHash = models.EmptyTreeCommitHash
	startHash     = "START"
)

// GraphRows lays out the graph of commits, newest first, a row of pipes
// per commit.
func GraphRows(commits []*models.Commit) [][]Pipe {
	if len(commits) == 0 {
		return nil
	}
	pipes := []Pipe{{From: 0, To: 0, fromHash: &startHash, toHash: commits[0].HashPtr(), Kind: Starts}}
	rows := make([][]Pipe, len(commits))
	for i, c := range commits {
		pipes = nextPipes(pipes, c)
		rows[i] = pipes
	}
	return rows
}

// Visible reports whether a pipe shows: not the one into the first row, nor
// one from a root commit to the empty tree.
func (p Pipe) Visible() bool {
	return p.fromHash != &startHash && p.toHash != &emptyTreeHash
}

// CommitColumn is the column of a row's commit.
func CommitColumn(row []Pipe) int16 {
	for _, p := range row {
		if p.Kind == Starts {
			return p.From
		}
	}
	return 0
}

func colorOf(c *models.Commit) uint8 {
	h := fnv.New32a()
	h.Write([]byte(c.Hash()))
	return uint8(h.Sum32() % 251)
}

func nextPipes(prevPipes []Pipe, commit *models.Commit) []Pipe {
	// A pipe that terminated in the previous row has no bearing on this one,
	// nor has one from a root commit to the empty tree.
	current := make([]Pipe, 0, len(prevPipes))
	for _, p := range prevPipes {
		if p.Kind != Terminates && p.toHash != &emptyTreeHash {
			current = append(current, p)
		}
	}

	maxPos := int16(-1)
	for _, p := range current {
		maxPos = max(maxPos, p.To)
	}

	newPipes := make([]Pipe, 0, len(current)+len(commit.ParentPtrs()))
	// A commit no earlier one leads to (as with log --all) goes at the end.
	pos := maxPos + 1
	for _, p := range current {
		if p.toHash == commit.HashPtr() {
			pos = p.To
			break
		}
	}

	taken := map[int16]bool{}     // spots a pipe ends on
	traversed := map[int16]bool{} // spots a pipe starts, ends or passes on

	toHash := &emptyTreeHash
	if !commit.IsFirstCommit() {
		toHash = commit.ParentPtrs()[0]
	}
	color := colorOf(commit)
	newPipes = append(newPipes, Pipe{From: pos, To: pos, fromHash: commit.HashPtr(), toHash: toHash, Kind: Starts, Color: color})

	traversedByContinuing := map[int16]bool{}
	for _, p := range current {
		if p.toHash != commit.HashPtr() {
			traversedByContinuing[p.To] = true
		}
	}
	nextForContinuing := func() int16 {
		i := int16(0)
		for traversed[i] {
			i++
		}
		return i
	}
	nextForNew := func() int16 {
		i := int16(0)
		for taken[i] || traversedByContinuing[i] {
			i++
		}
		return i
	}
	traverse := func(from, to int16) {
		for i := min(from, to); i <= max(from, to); i++ {
			traversed[i] = true
		}
		taken[to] = true
	}

	for _, p := range current {
		if p.toHash == commit.HashPtr() {
			newPipes = append(newPipes, Pipe{From: p.To, To: pos, fromHash: p.fromHash, toHash: p.toHash, Kind: Terminates, Color: p.Color})
			traverse(p.To, pos)
		} else if p.To < pos {
			at := nextForContinuing()
			newPipes = append(newPipes, Pipe{From: p.To, To: at, fromHash: p.fromHash, toHash: p.toHash, Kind: Continues, Color: p.Color})
			traverse(p.To, at)
		}
	}

	if commit.IsMerge() {
		for _, parent := range commit.ParentPtrs()[1:] {
			at := nextForNew()
			newPipes = append(newPipes, Pipe{From: pos, To: at, fromHash: commit.HashPtr(), toHash: parent, Kind: Starts, Color: color})
			taken[at] = true
		}
	}

	for _, p := range current {
		if p.toHash != commit.HashPtr() && p.To > pos {
			// Moving left into a free spot, if there is one.
			last := p.To
			for i := p.To; i > pos; i-- {
				if taken[i] || traversed[i] {
					break
				}
				last = i
			}
			newPipes = append(newPipes, Pipe{From: p.To, To: last, fromHash: p.fromHash, toHash: p.toHash, Kind: Continues, Color: p.Color})
			traverse(p.To, last)
		}
	}

	slices.SortFunc(newPipes, func(a, b Pipe) int {
		if a.To == b.To {
			return cmp.Compare(a.Kind, b.Kind)
		}
		return cmp.Compare(a.To, b.To)
	})
	return newPipes
}
