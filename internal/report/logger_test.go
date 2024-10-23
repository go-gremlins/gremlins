package report_test

import (
	"bytes"
	"errors"
	"reflect"
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
		t.Errorf("%s", cmp.Diff(got, want))
	}
}
