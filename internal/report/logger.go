// Package report formats and outputs mutation testing results.
package report

import (
	"errors"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// Filter maps mutation statuses to filter which mutants are logged.
type Filter = map[mutator.Status]struct{}

// ErrInvalidFilter is returned when an invalid status filter string is provided.
var ErrInvalidFilter = errors.New("invalid statuses filter, only 'lctkvsr' letters allowed")

// MutantLogger prints mutant statuses based on filter and verbosity flags.
type MutantLogger struct {
	Filter
}

// NewLogger creates a new MutantLogger with filters from configuration.
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

// Mutant logs a mutant if it passes the filter.
func (l MutantLogger) Mutant(m mutator.Mutator) {
	if l.Filter == nil {
		Mutant(m)

		return
	}

	if _, ok := l.Filter[m.Status()]; ok {
		Mutant(m)
	}
}

// ParseFilter parses a status filter string into a Filter map.
// Valid characters are 'lctkvsr' representing different mutation statuses.
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
