package diff

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/bluekeyes/go-gitdiff/gitdiff"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
)

func New() (Diff, error) {
	return NewWithCmd(exec.Command)
}

type execCmd interface {
	CombinedOutput() ([]byte, error)
}

func NewWithCmd[T execCmd](cmdContext func(name string, args ...string) T) (Diff, error) {
	diffRef := configuration.Get[string](configuration.UnleashDiffRef)
	if diffRef == "" {
		return nil, nil
	}

	log.Infoln("Gathering files diff...")

	cmd := cmdContext("git", "diff", "--merge-base", diffRef)

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
