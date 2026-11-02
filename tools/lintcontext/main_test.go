package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestRunJSON(t *testing.T) {
	t.Parallel()
	input := `{"a.go":"package p\nimport \"fmt\"\nfunc f() {}","empty.go":"package empty"}`
	var output bytes.Buffer
	if err := run(strings.NewReader(input), &output); err != nil {
		t.Fatalf("run() failed: %v", err)
	}
	var got map[string][]declaration
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("run() emitted invalid JSON: %v", err)
	}
	if len(got["a.go"]) != 2 || got["a.go"][1].Name != "func:f" || got["empty.go"] == nil {
		t.Errorf("run() = %+v, want import and function scopes plus an explicit empty array", got)
	}
}

func TestRunRejectsBadInputAndWriteFailure(t *testing.T) {
	t.Parallel()
	for _, input := range []string{"invalid JSON", `{"a.go":"package p\nfunc broken("}`} {
		if err := run(strings.NewReader(input), io.Discard); err == nil {
			t.Error("run() accepted invalid input")
		}
	}
	if err := run(strings.NewReader("{}"), failingWriter{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("run() = %v, want closed-pipe error", err)
	}
}

func TestDeclarationScopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name:   "functions",
			source: "package p\nfunc first() {}\nfunc second() {}",
			want:   []string{"func:first", "func:second"},
		},
		{
			name:   "generic receivers",
			source: "package p\nfunc (a *A[T]) Same() {}\nfunc (b *B[T]) Same() {}",
			want:   []string{"method:*A[T].Same", "method:*B[T].Same"},
		},
		{
			name:   "strings and comments",
			source: "package p\n// func fake() {}\nvar s = `func alsoFake() {}`\nfunc real() {}",
			want:   []string{"var:s", "func:real"},
		},
		{
			name:   "types and values",
			source: "package p\ntype A int\nvar a, b int\nconst n = 1",
			want:   []string{"type:A", "var:a,b", "const:n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := declarations(tt.source)
			if err != nil {
				t.Fatalf("declarations() failed: %v", err)
			}
			var names []string
			for _, item := range got {
				if item.Start < 1 || item.End < item.Start {
					t.Fatalf("invalid declaration range: %+v", item)
				}
				names = append(names, item.Name)
			}
			if !slices.Equal(names, tt.want) {
				t.Errorf("declarations() = %v, want %v", names, tt.want)
			}
		})
	}
}

func TestDeclarationScopesRejectInvalidSource(t *testing.T) {
	t.Parallel()
	if _, err := declarations("package p\nfunc broken("); err == nil {
		t.Fatal("declarations() accepted invalid source")
	}
}

func TestDeclarationScopesIncludeTypeDocumentation(t *testing.T) {
	t.Parallel()
	got, err := declarations("package p\n// First line.\n// Second line.\ntype T struct { X int }")
	if err != nil || len(got) != 1 || got[0].Start != 2 {
		t.Fatalf("declarations() = %+v, %v; want type documentation from line 2", got, err)
	}
}

func TestDeclarationScopesIncludeGroupedDocumentation(t *testing.T) {
	t.Parallel()
	got, err := declarations("package p\ntype (\n// A documentation.\nA int\n// B documentation.\nB int\n)")
	if err != nil || len(got) != 2 || got[0].Start != 3 || got[1].Start != 5 {
		t.Fatalf("declarations() = %+v, %v; want documentation from lines 3 and 5", got, err)
	}
}
