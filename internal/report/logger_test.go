package report_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/log"
	"github.com/go-gremlins/gremlins/internal/mutator"
	"github.com/go-gremlins/gremlins/internal/report"
)

func Test_parseFilter(t *testing.T) {
	tests := []struct {
		filter string
		want   report.Filter
		err    error
	}{
		{
			filter: "lc",
			want: report.Filter{
				mutator.Lived:      struct{}{},
				mutator.NotCovered: struct{}{},
			},
		},
		{
			filter: "tkvs",
			want: report.Filter{
				mutator.TimedOut:  struct{}{},
				mutator.Killed:    struct{}{},
				mutator.NotViable: struct{}{},
				mutator.Skipped:   struct{}{},
			},
		},
		{
			filter: "r",
			want: report.Filter{
				mutator.Runnable: struct{}{},
			},
		},
		{
			filter: "",
		},
		{
			filter: "lnc",
			want:   nil,
			err:    report.ErrInvalidFilter,
		},
	}
	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			got, err := report.ParseFilter(tt.filter)
			if !errors.Is(err, tt.err) {
				t.Errorf("ParseFilter() error = %v, wantErr %v", err, tt.err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFilter() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseDiffFilter(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input      string
		assertFunc func(t *testing.T, got report.Filter, err error)
	}{
		"should_return_nil_when_input_is_empty": {
			input: "",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil filter, got: %v", got)
				}
			},
		},
		"should_return_error_when_input_contains_invalid_character": {
			input: "x",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if !errors.Is(err, report.ErrInvalidDiffFilter) {
					t.Errorf("expected ErrInvalidDiffFilter, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil filter on error, got: %v", got)
				}
			},
		},
		"should_return_error_when_invalid_character_is_mixed_with_valid": {
			input: "lx",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if !errors.Is(err, report.ErrInvalidDiffFilter) {
					t.Errorf("expected ErrInvalidDiffFilter, got: %v", err)
				}
				if got != nil {
					t.Errorf("expected nil filter on error, got: %v", got)
				}
			},
		},
		"should_return_filter_with_lived_when_input_is_l": {
			input: "l",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				want := report.Filter{mutator.Lived: struct{}{}}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ParseDiffFilter() got = %v, want %v", got, want)
				}
			},
		},
		"should_return_filter_with_killed_when_input_is_k": {
			input: "k",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				want := report.Filter{mutator.Killed: struct{}{}}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ParseDiffFilter() got = %v, want %v", got, want)
				}
			},
		},
		"should_return_filter_with_both_statuses_when_input_is_lk": {
			input: "lk",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				want := report.Filter{
					mutator.Lived:  struct{}{},
					mutator.Killed: struct{}{},
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ParseDiffFilter() got = %v, want %v", got, want)
				}
			},
		},
		"should_deduplicate_repeated_characters": {
			input: "ll",
			assertFunc: func(t *testing.T, got report.Filter, err error) {
				t.Helper()
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				want := report.Filter{mutator.Lived: struct{}{}}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("ParseDiffFilter() got = %v, want %v", got, want)
				}
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := report.ParseDiffFilter(tc.input)
			tc.assertFunc(t, got, err)
		})
	}
}

func TestLogger(t *testing.T) {
	out := &bytes.Buffer{}
	defer out.Reset()
	log.Init(out, &bytes.Buffer{})
	defer log.Reset()

	m := stubMutant{status: mutator.NotCovered, mutantType: mutator.ConditionalsBoundary, position: fakePosition}

	configuration.Set(configuration.UnleashOutputStatusesKey, "lp")
	logger := report.NewLogger() // prints error

	logger.Mutant(m) // prints Not covered because no filter

	m.status = mutator.Killed

	configuration.Set(configuration.UnleashOutputStatusesKey, "")
	logger = report.NewLogger()

	logger.Mutant(m) // prints Killed because no filter

	configuration.Set(configuration.UnleashOutputStatusesKey, "l")
	logger = report.NewLogger()

	logger.Mutant(m) // Killed filtered

	m.status = mutator.Lived

	logger.Mutant(m) // prints Lived because no filter

	got := out.String()

	want := "output-statuses filter not applied: " + report.ErrInvalidFilter.Error() + "\n" +
		" NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"      KILLED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n"

	if !cmp.Equal(got, want) {
		t.Error(cmp.Diff(got, want))
	}
}

func TestLoggerOutputDiffStatuses(t *testing.T) {
	livedWithSnippets := stubMutant{
		status:         mutator.Lived,
		mutantType:     mutator.ConditionalsBoundary,
		position:       fakePosition,
		originSnippet:  []byte("x > y\n"),
		mutatedSnippet: []byte("x >= y\n"),
	}
	statusLine := "       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n"

	t.Run("prints diff when status matches output-diff-statuses", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()
		configuration.Set(configuration.UnleashOutputDiffStatusesKey, "l")
		defer configuration.Reset()

		logger := report.NewLogger()
		logger.Mutant(livedWithSnippets)

		got := out.String()
		if !strings.HasPrefix(got, statusLine) {
			t.Errorf("expected status line prefix, got: %q", got)
		}
		if !strings.Contains(got, "-x > y") || !strings.Contains(got, "+x >= y") {
			t.Errorf("expected diff lines in output, got: %q", got)
		}
	})

	t.Run("does not print diff when status does not match output-diff-statuses", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()
		configuration.Set(configuration.UnleashOutputDiffStatusesKey, "k")
		defer configuration.Reset()

		logger := report.NewLogger()
		logger.Mutant(livedWithSnippets)

		got := out.String()
		if got != statusLine {
			t.Errorf("expected only status line, got: %q", got)
		}
	})

	t.Run("does not print diff when output-diff-statuses is not set", func(t *testing.T) {
		out := &bytes.Buffer{}
		log.Init(out, &bytes.Buffer{})
		defer log.Reset()

		logger := report.NewLogger()
		logger.Mutant(livedWithSnippets)

		got := out.String()
		if got != statusLine {
			t.Errorf("expected only status line, got: %q", got)
		}
	})
}
