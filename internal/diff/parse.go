package diff

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
)

// New creates a new Diff by parsing git diff output using the default
// command executor.
//
// target is the directory Gremlins was pointed at, relative to the
// directory it is running in — "." for a whole module, "internal/api"
// for a scoped run. It decides which paths the diff is expressed in;
// see NewWithCmd.
func New(target string) (Diff, error) {
	return NewWithCmd(exec.Command, target)
}

type execCmd interface {
	CombinedOutput() ([]byte, error)
}

// NewWithCmd creates a new Diff by parsing git diff output using a
// custom command executor. This is useful for testing.
//
// The diff has to be expressed in the same paths as the mutant
// positions it will be compared against, and those are relative to
// whatever directory Gremlins was pointed at: a run over "." reports
// "internal/api/file.go", a run over "internal/api" reports "file.go".
// git, left alone, reports paths relative to the REPOSITORY root — so
// the two agree only when the module is the repository root and the
// target is ".".
//
// Everywhere else the lookup in Diff.IsChanged misses on every file,
// and the failure is silent in the direction that matters: every mutant
// comes back SKIPPED, the run finds nothing on the changed lines, and a
// --diff gate reports success having tested nothing. Measured against a
// Go module one directory below its repository root: 72 mutants, 72
// SKIPPED, including the line the diff had just changed.
//
// `git -C <target> diff --relative` fixes both halves at once. -C makes
// git run in the target directory and --relative makes it emit paths
// relative to that directory, which is exactly what the positions are
// relative to; and --relative also drops files outside the target,
// which is what a scoped run wants anyway.
func NewWithCmd[T execCmd](cmdContext func(name string, args ...string) T, target string) (Diff, error) {
	diffRef := configuration.Get[string](configuration.UnleashDiffRef)
	if diffRef == "" {
		return nil, nil
	}

	log.Infoln("Gathering files diff...")

	// --relative, because gremlins reports a mutant's position relative
	// to the module it is mutating while git reports a diff relative to
	// the repository root. Those coincide only when the module IS the
	// repository root. For a module in a subdirectory — a monorepo, a
	// Go workspace — every path in the diff carries a prefix the mutant
	// positions do not have, so the lookup in Diff.IsChanged misses on
	// every file.
	//
	// The failure is silent and it fails GREEN in the sense that
	// matters: every mutant is reported SKIPPED, the run finds nothing
	// on the changed lines, and a --diff gate passes having tested
	// nothing. Measured against a backend module one directory below
	// its repository root: 72 mutants, 72 SKIPPED, including the line
	// the diff had just changed.
	//
	// --relative makes git emit paths relative to the working directory
	// instead, which is the directory gremlins was invoked in and the
	// one its positions are relative to.
	cmd := cmdContext("git", "-C", target, "diff", "--relative", "--merge-base", diffRef)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("an error occured while calling git diff: %w\n\n%s", err, out)
	}

	files, _, err := gitdiff.Parse(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("an error occured while parsing diff: %w", err)
	}

	return newDiff(files), nil
}
