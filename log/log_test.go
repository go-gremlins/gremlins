/*
 * Copyright 2022 The Gremlins Authors
 *
 *    Licensed under the Apache License, Version 2.0 (the "License");
 *    you may not use this file except in compliance with the License.
 *    You may obtain a copy of the License at
 *
 *        http://www.apache.org/licenses/LICENSE-2.0
 *
 *    Unless required by applicable law or agreed to in writing, software
 *    distributed under the License is distributed on an "AS IS" BASIS,
 *    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *    See the License for the specific language governing permissions and
 *    limitations under the License.
 */

package log_test

import (
	"bytes"
	"github.com/google/go-cmp/cmp"
	"github.com/k3rn31/gremlins/log"
	"github.com/k3rn31/gremlins/mutant"
	"go/token"
	"testing"
)

func TestUninitialised(t *testing.T) {
	t.Parallel()
	out := &bytes.Buffer{}
	defer out.Reset()
	log.Init(out)
	log.Reset()

	log.Infof("%s", "test")
	log.Infoln("test")
	log.Errorf("%s", "test")
	log.Errorln("test")

	if out.String() != "" {
		t.Errorf("expected empty string")
	}
}

func TestLogInfo(t *testing.T) {
	out := &bytes.Buffer{}
	log.Init(out)
	t.Run("Infof", func(t *testing.T) {
		defer out.Reset()

		log.Infof("test %d", 1)

		got := out.String()

		want := "test 1"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}
	})

	t.Run("Infoln", func(t *testing.T) {
		defer out.Reset()

		log.Infoln("test test")

		got := out.String()

		want := "test test\n"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}
	})
	log.Reset()
}

func TestLogError(t *testing.T) {
	t.Run("Errorf", func(t *testing.T) {
		out := &bytes.Buffer{}
		defer out.Reset()
		log.Init(out)
		defer log.Reset()

		log.Errorf("test %d", 1)

		got := out.String()

		want := "ERROR: test 1"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}
	})

	t.Run("Errorln", func(t *testing.T) {
		out := &bytes.Buffer{}
		defer out.Reset()
		log.Init(out)
		defer log.Reset()

		log.Errorln("test test")

		got := out.String()

		want := "ERROR: test test\n"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}
	})
}

func TestMutantLog(t *testing.T) {
	out := &bytes.Buffer{}
	defer out.Reset()
	log.Init(out)
	defer log.Reset()

	m := stubMutant{mutant.Lived}
	log.Mutant(m)
	m = stubMutant{mutant.Killed}
	log.Mutant(m)
	m = stubMutant{mutant.NotCovered}
	log.Mutant(m)
	m = stubMutant{mutant.Runnable}
	log.Mutant(m)

	got := out.String()

	want := "" +
		"       LIVED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"      KILLED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		" NOT COVERED CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n" +
		"    RUNNABLE CONDITIONALS_BOUNDARY at aFolder/aFile.go:12:3\n"

	if !cmp.Equal(got, want) {
		t.Errorf(cmp.Diff(got, want))
	}
}

type stubMutant struct {
	status mutant.Status
}

func (s stubMutant) Type() mutant.Type {
	return mutant.ConditionalsBoundary
}

func (s stubMutant) SetType(_ mutant.Type) {
	panic("implement me")
}

func (s stubMutant) Status() mutant.Status {
	return s.status
}

func (s stubMutant) SetStatus(_ mutant.Status) {
	panic("implement me")
}

func (s stubMutant) Position() token.Position {
	return token.Position{
		Filename: "aFolder/aFile.go",
		Offset:   0,
		Line:     12,
		Column:   3,
	}
}

func (s stubMutant) Pos() token.Pos {
	return 123
}

func (s stubMutant) SetWorkdir(_ string) {
	panic("implement me")
}

func (s stubMutant) Apply() error {
	panic("implement me")
}

func (s stubMutant) Rollback() error {
	panic("implement me")
}
