package diff

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
)

// New creates a new Diff by parsing git diff output using the default command executor.
// moduleRoot is the absolute path to the Go module root (mod.Root).
// callingDir is the relative path from the module root to the CWD (mod.CallingDir).
func New(moduleRoot, callingDir string) (Diff, error) {
	return NewWithCmd(exec.Command, moduleRoot, callingDir)
}

type execCmd interface {
	CombinedOutput() ([]byte, error)
}

// NewWithCmd creates a new Diff by parsing git diff output using a custom command executor.
// This is useful for testing.
func NewWithCmd[T execCmd](cmdContext func(name string, args ...string) T, moduleRoot, callingDir string) (Diff, error) {
	diffRef := configuration.Get[string](configuration.UnleashDiffRef)
	if diffRef == "" {
		return Diff{}, nil
	}

	log.Infoln("Gathering files diff...")

	absModuleRoot, err := filepath.Abs(moduleRoot)
	if err != nil {
		return Diff{}, fmt.Errorf("failed to resolve module root: %w", err)
	}

	gitRootOut, err := cmdContext("git", "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return Diff{}, fmt.Errorf("failed to determine git root: %w", err)
	}
	gitRoot := strings.TrimSpace(string(gitRootOut))

	moduleRel, err := filepath.Rel(gitRoot, absModuleRoot)
	if err != nil {
		return Diff{}, fmt.Errorf("failed to determine module path relative to git root: %w", err)
	}

	cmd := cmdContext("git", "diff", "--merge-base", diffRef)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return Diff{}, fmt.Errorf("an error occured while calling git diff: %w\n\n%s", err, out)
	}

	files, _, err := gitdiff.Parse(bytes.NewReader(out))
	if err != nil {
		return Diff{}, fmt.Errorf("an error occured while parsing diff: %w", err)
	}

	return newDiff(files, moduleRel, callingDir), nil
}
