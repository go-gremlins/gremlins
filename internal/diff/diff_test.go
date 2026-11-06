package diff

import (
	"go/token"
	"reflect"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const (
	testFile  = "test"
	testFile1 = "test1"
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
			name: "must be unchanged on empty Diff",
			d:    map[FileName][]Change{},
			pos:  token.Position{},
			want: false,
		},
		{
			name: "must be changed if in range",
			d: map[FileName][]Change{
				testFile: {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: testFile, Line: 21},
			want: true,
		},
		{
			name: "must be unchanged if outside range",
			d: map[FileName][]Change{
				testFile: {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: testFile, Line: 22},
			want: false,
		},
		{
			name: "must be unchanged if no such file",
			d: map[FileName][]Change{
				testFile: {{StartLine: 21, EndLine: 21}},
			},
			pos:  token.Position{Filename: testFile1, Line: 21},
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
			NewName:       testFile1,
			TextFragments: fragments,
		},
		{
			NewName:       "test2",
			TextFragments: fragments,
		},
	}

	expected := Diff{
		testFile1: {{StartLine: 25, EndLine: 25}},
		"test2":   {{StartLine: 25, EndLine: 25}},
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
	}
	file := &gitdiff.File{
		NewName:       testFile,
		TextFragments: fragments,
	}

	expect := []Change{
		{StartLine: 4, EndLine: 4},
		{StartLine: 25, EndLine: 26},
		{StartLine: 48, EndLine: 51},
		{StartLine: 235, EndLine: 435},
	}

	name, changes := newChanges(file)

	if name != testFile {
		t.Fatalf("name %s unexpected", name)
	}
	if !reflect.DeepEqual(changes, expect) {
		t.Log("want", expect)
		t.Log("got", changes)
		t.Fatalf("unexpected newChanges result")
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

func Test_newChanges_separateEditsInOneFragment(t *testing.T) {
	var lines []gitdiff.Line
	lines = append(lines, opLines(gitdiff.OpContext, 3)...)
	lines = append(lines, opLines(gitdiff.OpDelete, 1)...)
	lines = append(lines, opLines(gitdiff.OpAdd, 1)...)
	lines = append(lines, opLines(gitdiff.OpContext, 5)...)
	lines = append(lines, opLines(gitdiff.OpDelete, 1)...)
	lines = append(lines, opLines(gitdiff.OpAdd, 1)...)
	lines = append(lines, opLines(gitdiff.OpContext, 3)...)

	file := &gitdiff.File{
		NewName: testFile,
		TextFragments: []*gitdiff.TextFragment{
			{
				NewPosition: 5,
				Lines:       lines,
			},
		},
	}

	expect := []Change{
		{StartLine: 8, EndLine: 8},
		{StartLine: 14, EndLine: 14},
	}

	name, changes := newChanges(file)

	if name != testFile {
		t.Fatalf("name %s unexpected", name)
	}
	if !reflect.DeepEqual(changes, expect) {
		t.Log("want", expect)
		t.Log("got", changes)
		t.Fatal("unexpected newChanges result for a hunk with two separate edits")
	}
}
