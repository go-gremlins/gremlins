package diff

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
)

// New creates a new Diff by parsing git diff output using the default command
// executor. moduleRoot is the root of the Go module under test; see
// NewWithCmd for why the diff is read from there.
func New(moduleRoot string) (Diff, error) {
	return NewWithCmd(exec.Command, moduleRoot)
}

type execCmd interface {
	CombinedOutput() ([]byte, error)
}

// NewWithCmd creates a new Diff by parsing git diff output using a custom command executor.
// This is useful for testing.
//
// The diff is read from moduleRoot with --relative, so every file name in it
// is relative to the module root, the same base the engine resolves its
// positions against. Plain `git diff` names files relative to the repository
// root whatever the working directory, which matches nothing once the module
// is not the repository root or the run is scoped to a subdirectory.
func NewWithCmd[T execCmd](cmdContext func(name string, args ...string) T, moduleRoot string) (Diff, error) {
	diffRef := configuration.Get[string](configuration.UnleashDiffRef)
	if diffRef == "" {
		return nil, nil
	}

	log.Infoln("Gathering files diff...")

	cmd := cmdContext("git", "-C", moduleRoot, "diff", "--merge-base", "--relative", diffRef)

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
