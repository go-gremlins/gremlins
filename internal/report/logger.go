package report

import (
	"errors"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

type Filter = map[mutator.Status]struct{}

var ErrInvalidFilter = errors.New("invalid statuses filter, only 'lctkvsr' letters allowed")

// MutantLogger prints mutant statuses based on filter and verbosity flags.
type MutantLogger struct {
	Filter
}

func NewLogger() MutantLogger {
	outputStatuses := configuration.Get[string](configuration.UnleashOutputStatusesKey)
	f, err := ParseFilter(outputStatuses)
	if err != nil {
		log.Infof("output-statuses filter not applied: %s\n", err)
	}

	return MutantLogger{
		Filter: f,
	}
}

func (l MutantLogger) Mutant(m mutator.Mutator) {
	if l.Filter == nil {
		Mutant(m)

		return
	}

	if _, ok := l.Filter[m.Status()]; ok {
		Mutant(m)
	}
}

func ParseFilter(s string) (Filter, error) {
	if s == "" {
		return nil, nil
	}

	result := Filter{}

	for _, r := range s {
		switch r {
		case 'l':
			result[mutator.Lived] = struct{}{}
		case 'c':
			result[mutator.NotCovered] = struct{}{}
		case 't':
			result[mutator.TimedOut] = struct{}{}
		case 'k':
			result[mutator.Killed] = struct{}{}
		case 'v':
			result[mutator.NotViable] = struct{}{}
		case 's':
			result[mutator.Skipped] = struct{}{}
		case 'r':
			result[mutator.Runnable] = struct{}{}
		default:
			return nil, ErrInvalidFilter
		}
	}

	return result, nil
}
