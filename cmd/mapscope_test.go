package cmd

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/diff"
	"github.com/go-gremlins/gremlins/internal/gomodule"
)

const (
	scopeModule = "example.com"
	apiPackage  = "example.com/internal/api"
)

func TestMapScope(t *testing.T) {
	mod := gomodule.GoModule{Name: scopeModule, Root: ".", CallingDir: "."}
	noDependents := func(string) []string { return nil }

	t.Run("no diff maps everything", func(t *testing.T) {
		if got := mapScope(mod, nil, noDependents); got != nil {
			t.Error("want no scope when the run has no diff")
		}
	})

	// Mutants live only in changed production files, so only their packages
	// need a map; a changed test or a non-Go file holds none, and neither does a
	// file under testdata or vendor, which gremlins never mutates (#291).
	changes := diff.Diff{
		"internal/api/handler.go":            nil,
		"internal/api/handler_test.go":       nil,
		"internal/store/store_test.go":       nil,
		"internal/store/testdata/fixture.go": nil,
		"vendor/example.org/lib/lib.go":      nil,
		"main.go":                            nil,
		"README.md":                          nil,
	}

	t.Run("the packages of the changed production files", func(t *testing.T) {
		want := []string{scopeModule, apiPackage}
		if diff := cmp.Diff(want, mapScope(mod, changes, noDependents)); diff != "" {
			t.Errorf("mapScope mismatch (-want +got):\n%s", diff)
		}
	})

	// Diff paths are relative to the directory gremlins was pointed at.
	t.Run("from a calling directory below the module root", func(t *testing.T) {
		sub := gomodule.GoModule{Name: scopeModule, Root: ".", CallingDir: "internal"}
		want := []string{apiPackage}
		if diff := cmp.Diff(want, mapScope(sub, diff.Diff{"api/handler.go": nil}, noDependents)); diff != "" {
			t.Errorf("mapScope mismatch (-want +got):\n%s", diff)
		}
	})

	// With --cross-package a dependent's tests are selected for the mutant,
	// and selection reads them from the map, so the dependents need mapping too.
	t.Run("with dependents, theirs too", func(t *testing.T) {
		dependents := func(pkg string) []string {
			if pkg == apiPackage {
				return []string{"example.com/cmd/server"}
			}

			return nil
		}
		want := []string{scopeModule, "example.com/cmd/server", apiPackage}
		if diff := cmp.Diff(want, mapScope(mod, changes, dependents)); diff != "" {
			t.Errorf("mapScope mismatch (-want +got):\n%s", diff)
		}
	})
}
