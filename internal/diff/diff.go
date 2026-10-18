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

// newChanges records each run of consecutive added lines as its own Change.
// A fragment is not one run: it interleaves added lines with context and
// deleted lines, so a range taken from its first added line and its added
// count misses every added line after the first gap.
func newChanges(file *gitdiff.File) (FileName, []Change) {
	var changes []Change

	for _, fragment := range file.TextFragments {
		line := int(fragment.NewPosition)
		for _, l := range fragment.Lines {
			switch l.Op {
			case gitdiff.OpAdd:
				if n := len(changes); n > 0 && changes[n-1].EndLine == line-1 {
					changes[n-1].EndLine = line
				} else {
					changes = append(changes, Change{StartLine: line, EndLine: line})
				}
				line++
			case gitdiff.OpContext:
				line++
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
