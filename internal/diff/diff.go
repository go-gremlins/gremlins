// Package diff parses git diff output to identify changed lines for incremental mutation testing.
package diff

import (
	"go/token"
	"path"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

// FileName represents a file path in a diff.
type FileName string

// Change represents a contiguous range of changed lines in a file.
type Change struct {
	// StartLine is the first line of the changed region (1-indexed).
	StartLine int
	// EndLine is the last line of the changed region (1-indexed, inclusive).
	EndLine int
}

// Diff maps file names to their list of changes and carries path
// information for monorepo support.
//
// moduleRel is the relative path from the git repo root to the Go module root
// (e.g. "service-a"). callingDir is the relative path from the module root
// to the current working directory. These are prepended to pos.Filename
// during IsChanged lookups so that git-root-relative diff keys match
// module-root-relative token positions.
type Diff struct {
	changes    map[FileName][]Change
	moduleRel  string
	callingDir string
}

// FromChanges creates a Diff from a pre-built changes map, bypassing git diff.
// This is used in tests and when the caller already has a changes map.
func FromChanges(changes map[FileName][]Change) Diff {
	return Diff{changes: changes}
}

// WithModuleRel sets the module relative path (from git root to module root).
func (d Diff) WithModuleRel(rel string) Diff {
	return Diff{
		changes:    d.changes,
		moduleRel:  rel,
		callingDir: d.callingDir,
	}
}

// WithCallingDir sets the calling directory (from module root to CWD).
func (d Diff) WithCallingDir(dir string) Diff {
	return Diff{
		changes:    d.changes,
		moduleRel:  d.moduleRel,
		callingDir: dir,
	}
}

func newDiff(files []*gitdiff.File, moduleRel, callingDir string) Diff {
	result := map[FileName][]Change{}

	for _, file := range files {
		name, changes := newChanges(file)

		result[name] = changes
	}

	return Diff{
		changes:    result,
		moduleRel:  moduleRel,
		callingDir: callingDir,
	}
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

// IsChanged returns true if the given position is within a changed region.
// If the diff is empty, it returns true for all positions.
func (d Diff) IsChanged(pos token.Position) bool {
	if len(d.changes) == 0 {
		return true
	}

	// Diff keys always use "/" (git's own convention), regardless of OS, so the
	// prefix is joined with path.Join rather than filepath.Join, and each part is
	// normalised to "/" in case it was built with filepath.Rel/filepath.Join on Windows.
	key := FileName(path.Join(toSlash(d.moduleRel), toSlash(d.callingDir), toSlash(pos.Filename)))
	fileDiff := d.changes[key]

	for _, change := range fileDiff {
		if pos.Line >= change.StartLine && pos.Line <= change.EndLine {
			return true
		}
	}

	return false
}

// toSlash normalises path separators to "/", regardless of the host OS,
// since diff keys are always built from git's forward-slash paths.
func toSlash(s string) string {
	return strings.ReplaceAll(s, `\`, "/")
}
