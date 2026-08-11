package diff

import (
	"go/token"
	"reflect"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

func TestDiff_IsChanged(t *testing.T) {
	tests := []struct {
		name string
		d    Diff
		pos  token.Position
		want bool
	}{
		{
			name: "must be changed on nil Diff",
			d:    nil,
			pos:  token.Position{},
			want: true,
		},
		{
			name: "must be changed on empty Diff",
			d:    map[FileName][]Change{},
			pos:  token.Position{},
			want: true,
		},
		{
			name: "must be changed if in range",
			d: map[FileName][]Change{
				"test": {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: "test", Line: 21},
			want: true,
		},
		{
			name: "must be unchanged if outside range",
			d: map[FileName][]Change{
				"test": {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: "test", Line: 22},
			want: false,
		},
		{
			name: "must be unchanged if no such file",
			d: map[FileName][]Change{
				"test": {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: "test1", Line: 21},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.d.IsChanged(tt.pos)
			if got != tt.want {
				t.Errorf("IsChanged() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newDiff(t *testing.T) {
	fragments := []*gitdiff.TextFragment{fragment(21, 1)}

	files := []*gitdiff.File{
		{
			NewName:       "test1",
			TextFragments: fragments,
		},
		{
			NewName:       "test2",
			TextFragments: fragments,
		},
	}

	expected := Diff{
		"test1": {{StartLine: 25, EndLine: 25}},
		"test2": {{StartLine: 25, EndLine: 25}},
	}

	result := newDiff(files)
	if !reflect.DeepEqual(result, expected) {
		t.Log("want", expected)
		t.Log("got", result)
		t.Fatalf("unexpected newDiff result")
	}
}

func Test_newChanges(t *testing.T) {
	fragments := []*gitdiff.TextFragment{
		fragment(0, 1),
		fragment(10, 0),
		fragment(21, 2),
		fragment(44, 4),
		fragment(231, 201),
		multiRunFragment(600,
			lineSegment{gitdiff.OpContext, 4},
			lineSegment{gitdiff.OpDelete, 2},
			lineSegment{gitdiff.OpAdd, 2},
			lineSegment{gitdiff.OpContext, 3},
			lineSegment{gitdiff.OpDelete, 2},
			lineSegment{gitdiff.OpAdd, 3},
			lineSegment{gitdiff.OpContext, 4},
		),
	}
	file := &gitdiff.File{
		NewName:       "test",
		TextFragments: fragments,
	}

	expect := []Change{
		{StartLine: 4, EndLine: 4},
		{StartLine: 25, EndLine: 26},
		{StartLine: 48, EndLine: 51},
		{StartLine: 235, EndLine: 435},
		{StartLine: 604, EndLine: 605},
		{StartLine: 609, EndLine: 611},
	}

	name, changes := newChanges(file)

	if name != "test" {
		t.Fatalf("name %s unexpected", name)
	}
	if !reflect.DeepEqual(changes, expect) {
		t.Log("want", expect)
		t.Log("got", changes)
		t.Fatalf("unexpected newChanges result")
	}
}

// lineSegment describes a run of lines sharing the same diff operation,
// used to build fragments with more than one added-line run.
type lineSegment struct {
	op    gitdiff.LineOp
	count int
}

// multiRunFragment builds a fragment from an arbitrary ordered sequence of
// context/delete/add segments, unlike fragment which always places a single
// delete run immediately before a single add run. It reproduces hunks where
// git's context radius merges two separate, non-contiguous edits into one
// fragment.
func multiRunFragment(startLine int, segments ...lineSegment) *gitdiff.TextFragment {
	var lines []gitdiff.Line

	var added, deleted int64

	for _, seg := range segments {
		lines = append(lines, opLines(seg.op, seg.count)...)

		switch seg.op {
		case gitdiff.OpAdd:
			added += int64(seg.count)
		case gitdiff.OpDelete:
			deleted += int64(seg.count)
		case gitdiff.OpContext:
		}
	}

	var leadingContext, trailingContext int64
	if len(segments) > 0 && segments[0].op == gitdiff.OpContext {
		leadingContext = int64(segments[0].count)
	}

	if len(segments) > 0 && segments[len(segments)-1].op == gitdiff.OpContext {
		trailingContext = int64(segments[len(segments)-1].count)
	}

	line := int64(startLine)

	return &gitdiff.TextFragment{
		OldLines:        line - 1,
		NewPosition:     line,
		LinesAdded:      added,
		LinesDeleted:    deleted,
		LeadingContext:  leadingContext,
		TrailingContext: trailingContext,
		Lines:           lines,
	}
}

func fragment(startLine int, adds int, del ...int) *gitdiff.TextFragment {
	const contexts = 4

	dels := adds
	if len(del) > 0 {
		dels = del[0]
	}

	var lines []gitdiff.Line

	lines = append(lines, opLines(gitdiff.OpContext, contexts)...)
	lines = append(lines, opLines(gitdiff.OpDelete, dels)...)
	lines = append(lines, opLines(gitdiff.OpAdd, adds)...)
	lines = append(lines, opLines(gitdiff.OpContext, contexts)...)

	line := int64(startLine)
	added := int64(adds)
	deleted := int64(dels)

	return &gitdiff.TextFragment{
		OldLines:        line - 1,
		NewPosition:     line,
		LinesAdded:      added,
		LinesDeleted:    deleted,
		LeadingContext:  contexts,
		TrailingContext: contexts,
		Lines:           lines,
	}
}

func opLines(op gitdiff.LineOp, count int) []gitdiff.Line {
	result := make([]gitdiff.Line, count)

	for i := 0; i < count; i++ {
		result[i] = gitdiff.Line{Op: op, Line: "test"}
	}

	return result
}
