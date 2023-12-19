package diff

import (
	"go/token"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type FileName string

type Change struct {
	StartLine int
	EndLine   int
}

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

		startLine := int(fragment.NewPosition + fragment.LeadingContext)

		changes = append(changes, Change{
			StartLine: startLine,
			EndLine:   startLine + int(fragment.LinesAdded-1),
		})
	}

	return FileName(file.NewName), changes
}

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
