// Package diff parses git diff output to identify changed lines for incremental mutation testing.
package diff

import (
	"go/token"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// FileName represents a file path in a diff.
type FileName string

// Change represents a contiguous range of changed lines in a file.
type Change struct {
	StartLine int
	EndLine   int
}

// Diff maps file names to their list of changes.
type Diff map[FileName][]Change

func newDiff(files []*gitdiff.File) Diff {
	result := map[FileName][]Change{}

	for _, file := range files {
		name, changes := newChanges(file)

		result[name] = changes
	}

	return result
}

// newChanges walks a fragment line by line to find the new-file line numbers
// that were actually added.
//
// It cannot be derived from the fragment header. An earlier version computed
// one range per fragment as NewPosition+LeadingContext .. +LinesAdded-1, which
// assumes every added line sits in a single contiguous block right after the
// leading context. A fragment holding TWO blocks of additions -- the shape any
// edit produces when it inserts something and also modifies a line further
// down, and the default 3 lines of context routinely merge those into one
// fragment -- then had its later blocks silently dropped, while unchanged
// context lines between the blocks were wrongly reported as changed.
//
// Concretely, for a fragment adding new-file lines 4, 5 and 7, the old
// arithmetic claimed 4..6: it included context line 6 and lost line 7. Under
// --diff that means a mutant on a genuinely changed line is SKIPPED and the
// run reports "0 mutants on changed lines", passing vacuously.
//
// Only OpAdd and OpContext advance the new-file line counter; OpDelete lines
// exist in the old file only.
func newChanges(file *gitdiff.File) (FileName, []Change) {
	var changes []Change

	for _, fragment := range file.TextFragments {
		line := int(fragment.NewPosition)
		cur := -1

		for _, fragLine := range fragment.Lines {
			switch fragLine.Op {
			case gitdiff.OpAdd:
				if cur >= 0 && changes[cur].EndLine == line-1 {
					changes[cur].EndLine = line
				} else {
					changes = append(changes, Change{StartLine: line, EndLine: line})
					cur = len(changes) - 1
				}

				line++
			case gitdiff.OpContext:
				line++
				cur = -1
			case gitdiff.OpDelete:
			}
		}
	}

	return FileName(file.NewName), changes
}

// IsChanged returns true if the given position is within a changed region.
// If the diff is empty, it returns true for all positions.
func (d Diff) IsChanged(pos token.Position) bool {
	if len(d) == 0 {
		return true
	}

	fileDiff := d[FileName(pos.Filename)]

	for _, change := range fileDiff {
		if pos.Line >= change.StartLine && pos.Line <= change.EndLine {
			return true
		}
	}

	return false
}
