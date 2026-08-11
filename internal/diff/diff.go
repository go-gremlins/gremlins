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

func newChanges(file *gitdiff.File) (FileName, []Change) {
	var changes []Change

	for _, fragment := range file.TextFragments {
		if fragment.LinesAdded == 0 {
			continue
		}

		newLine := fragment.NewPosition
		var current *Change

		for _, line := range fragment.Lines {
			switch line.Op {
			case gitdiff.OpAdd:
				if current == nil {
					current = &Change{StartLine: int(newLine), EndLine: int(newLine)}
				} else {
					current.EndLine = int(newLine)
				}
				newLine++
			case gitdiff.OpContext:
				if current != nil {
					changes = append(changes, *current)
					current = nil
				}
				newLine++
			case gitdiff.OpDelete:
				// Deleted lines don't exist in the new file and don't close
				// an open run: they can sit between two adds that together
				// form one replacement region.
			}
		}

		if current != nil {
			changes = append(changes, *current)
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
