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
		changes = append(changes, changesInFragment(fragment)...)
	}

	return FileName(file.NewName), changes
}

// changesInFragment walks a fragment's lines in order and returns one Change
// per contiguous run of added lines, tracked by the line's actual position in
// the new file. A hunk can hold more than one such run when git merges two
// nearby edits into a single fragment.
func changesInFragment(fragment *gitdiff.TextFragment) []Change {
	var changes []Change

	newLine := int(fragment.NewPosition)
	var current *Change

	for _, line := range fragment.Lines {
		switch {
		case line.Op != gitdiff.OpAdd:
			if current != nil {
				changes = append(changes, *current)
				current = nil
			}
		case current == nil:
			current = &Change{StartLine: newLine, EndLine: newLine}
		default:
			current.EndLine = newLine
		}

		if line.New() {
			newLine++
		}
	}

	if current != nil {
		changes = append(changes, *current)
	}

	return changes
}

// IsChanged returns true if the given position is within a changed region.
// A nil Diff means diff-based selection is disabled, so every position is
// changed. A non-nil Diff with no matching range means diff-based selection
// is enabled but found no added lines for that position, so it is unchanged.
func (d Diff) IsChanged(pos token.Position) bool {
	if d == nil {
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
