package diff

import (
	"go/token"
	"reflect"
	"testing"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

const (
	svcAPref = "service-a"
	svcAFile = svcAPref + "/main.go"
	flatFile = "main.go"
)

func TestDiff_IsChanged(t *testing.T) {
	tests := []struct {
		name string
		d    Diff
		pos  token.Position
		want bool
	}{
		{
			name: "must be changed on zero Diff",
			d:    Diff{},
			pos:  token.Position{},
			want: true,
		},
		{
			name: "must be changed on empty Diff",
			d:    Diff{changes: map[FileName][]Change{}},
			pos:  token.Position{},
			want: true,
		},
		{
			name: "must be changed if in range",
			d: Diff{
				changes: map[FileName][]Change{
					"test": {{StartLine: 21, EndLine: 21}},
				},
			},
			pos:  token.Position{Filename: "test", Line: 21},
			want: true,
		},
		{
			name: "must be unchanged if outside range",
			d: Diff{
				changes: map[FileName][]Change{
					"test": {{StartLine: 21, EndLine: 21}},
				},
			},
			pos:  token.Position{Filename: "test", Line: 22},
			want: false,
		},
		{
			name: "must be unchanged if no such file",
			d: Diff{
				changes: map[FileName][]Change{
					"test": {{StartLine: 21, EndLine: 21}},
				},
			},
			pos:  token.Position{Filename: "test1", Line: 21},
			want: false,
		},
		{
			name: "monorepo single service: prepend moduleRel",
			d: Diff{
				changes: map[FileName][]Change{
					svcAFile: {{StartLine: 10, EndLine: 20}},
				},
				moduleRel: svcAPref,
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "monorepo single service: no match outside range",
			d: Diff{
				changes: map[FileName][]Change{
					svcAFile: {{StartLine: 10, EndLine: 20}},
				},
				moduleRel: svcAPref,
			},
			pos:  token.Position{Filename: flatFile, Line: 25},
			want: false,
		},
		{
			name: "monorepo subdirectory: prepend moduleRel and callingDir",
			d: Diff{
				changes: map[FileName][]Change{
					"service-a/cmd/main.go": {{StartLine: 10, EndLine: 20}},
				},
				moduleRel:  svcAPref,
				callingDir: "cmd",
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "monorepo multiple services: no collision",
			d: Diff{
				changes: map[FileName][]Change{
					svcAFile:            {{StartLine: 10, EndLine: 20}},
					"service-b/main.go": {{StartLine: 30, EndLine: 40}},
				},
				moduleRel: svcAPref,
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "monorepo multiple services: other service not matched",
			d: Diff{
				changes: map[FileName][]Change{
					svcAFile:            {{StartLine: 10, EndLine: 20}},
					"service-b/main.go": {{StartLine: 30, EndLine: 40}},
				},
				moduleRel: "service-b",
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: false,
		},
		{
			name: "flat repo (moduleRel='.'): resolves to same key",
			d: Diff{
				changes: map[FileName][]Change{
					flatFile: {{StartLine: 10, EndLine: 20}},
				},
				moduleRel: ".",
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "empty callingDir is not prepended",
			d: Diff{
				changes: map[FileName][]Change{
					svcAFile: {{StartLine: 10, EndLine: 20}},
				},
				moduleRel: svcAPref,
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "windows-style separators in moduleRel/callingDir still match forward-slash diff keys",
			d: Diff{
				changes: map[FileName][]Change{
					"service-a/cmd/main.go": {{StartLine: 10, EndLine: 20}},
				},
				moduleRel:  `service-a`,
				callingDir: `cmd`,
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
		},
		{
			name: "backslash-joined moduleRel from filepath.Rel on Windows still matches",
			d: Diff{
				changes: map[FileName][]Change{
					"service-a/cmd/main.go": {{StartLine: 10, EndLine: 20}},
				},
				moduleRel: `service-a\cmd`,
			},
			pos:  token.Position{Filename: flatFile, Line: 15},
			want: true,
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
		changes: map[FileName][]Change{
			"test1": {{StartLine: 25, EndLine: 25}},
			"test2": {{StartLine: 25, EndLine: 25}},
		},
	}

	result := newDiff(files, "", "")
	if !reflect.DeepEqual(result, expected) {
		t.Log("want", expected)
		t.Log("got", result)
		t.Fatalf("unexpected newDiff result")
	}
}

func Test_newDiff_withModuleRel(t *testing.T) {
	fragments := []*gitdiff.TextFragment{fragment(21, 1)}

	files := []*gitdiff.File{
		{
			NewName:       "service-a/test1",
			TextFragments: fragments,
		},
		{
			NewName:       "service-b/test2",
			TextFragments: fragments,
		},
	}

	expected := Diff{
		changes: map[FileName][]Change{
			"service-a/test1": {{StartLine: 25, EndLine: 25}},
			"service-b/test2": {{StartLine: 25, EndLine: 25}},
		},
		moduleRel: svcAPref,
	}

	result := newDiff(files, svcAPref, "")
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
		NewName:       "test",
		TextFragments: fragments,
	}

	expect := []Change{
		{StartLine: 4, EndLine: 4},
		{StartLine: 25, EndLine: 26},
		{StartLine: 48, EndLine: 51},
		{StartLine: 235, EndLine: 435},
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

func TestFromChanges(t *testing.T) {
	d := FromChanges(map[FileName][]Change{
		"file.go": {{StartLine: 1, EndLine: 5}},
	})
	if d.changes == nil {
		t.Fatal("changes should not be nil")
	}
	if len(d.changes) != 1 {
		t.Fatalf("expected 1 file, got %d", len(d.changes))
	}
	if d.moduleRel != "" || d.callingDir != "" {
		t.Fatal("moduleRel and callingDir should be empty")
	}
}

func TestDiffWithModuleRel(t *testing.T) {
	d := FromChanges(map[FileName][]Change{
		svcAFile: {{StartLine: 10, EndLine: 20}},
	}).WithModuleRel(svcAPref)

	pos := token.Position{Filename: flatFile, Line: 15}
	if !d.IsChanged(pos) {
		t.Error("expected IsChanged to match with moduleRel prepended")
	}

	pos2 := token.Position{Filename: "other.go", Line: 15}
	if d.IsChanged(pos2) {
		t.Error("expected IsChanged to not match unrelated file")
	}
}

func TestDiffWithCallingDir(t *testing.T) {
	d := FromChanges(map[FileName][]Change{
		"service-a/cmd/main.go": {{StartLine: 10, EndLine: 20}},
	}).WithModuleRel(svcAPref).WithCallingDir("cmd")

	pos := token.Position{Filename: flatFile, Line: 15}
	if !d.IsChanged(pos) {
		t.Error("expected IsChanged to match with moduleRel and callingDir prepended")
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
