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
	"github.com/k3rn31/gremlins/log"
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
