package diff

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
)

// New creates a new Diff by parsing git diff output using the default command executor.
func New() (Diff, error) {
	return NewWithCmd(exec.Command)
}

type execCmd interface {
	CombinedOutput() ([]byte, error)
}

// NewWithCmd creates a new Diff by parsing git diff output using a custom command executor.
// This is useful for testing.
func NewWithCmd[T execCmd](cmdContext func(name string, args ...string) T) (Diff, error) {
	diffRef := configuration.Get[string](configuration.UnleashDiffRef)
	if diffRef == "" {
		return nil, nil
	}

	log.Infoln("Gathering files diff...")

	// --relative makes git emit paths relative to the current working directory
	// rather than the repository root. gremlins runs from a Go module root, which
	// in a monorepo is a repository subdirectory, and keys the diff by these paths
	// to match the module-relative token.Position filenames it parses; without
	// --relative the repo-root-relative keys never match and every mutant in a
	// subdirectory module is skipped.
	cmd := cmdContext("git", "diff", "--merge-base", diffRef, "--relative")

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
