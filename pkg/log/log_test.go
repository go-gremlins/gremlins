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
	"testing"

	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/pkg/log"
)

func TestUninitialised(t *testing.T) {
	out := &bytes.Buffer{}
	defer out.Reset()
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
	log.Init(out, &bytes.Buffer{})

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
	out := &bytes.Buffer{}
	eOut := &bytes.Buffer{}
	log.Init(out, eOut)
	defer log.Reset()

	t.Run("Errorf", func(t *testing.T) {
		defer out.Reset()
		defer eOut.Reset()

		log.Errorf("test %d", 1)

		got := eOut.String()

		want := "ERROR: test 1"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}

		got = out.String()
		if got != "" {
			t.Errorf("expected out to be empty, got %s", got)
		}
	})

	t.Run("Errorln", func(t *testing.T) {
		defer out.Reset()
		defer eOut.Reset()

		log.Errorln("test test")

		got := eOut.String()

		want := "ERROR: test test\n"
		if got != want {
			t.Errorf("want %q, got %q", want, got)
		}

		got = out.String()
		if got != "" {
			t.Errorf("expected out to be empty, got %s", got)
		}
	})
}

func TestSilentMode(t *testing.T) {
	viper.Set("silent", true)
	defer viper.Reset()

	sOut := &bytes.Buffer{}
	defer sOut.Reset()
	eOut := &bytes.Buffer{}
	defer eOut.Reset()
	log.Init(sOut, eOut)
	defer log.Reset()

	log.Infof("%s", "test")
	log.Infoln("test")
	log.Errorf("%s\n", "test")
	log.Errorln("test")

	if sOut.String() != "" {
		t.Errorf("expected empty string")
	}
	if eOut.String() != "ERROR: test\nERROR: test\n" {
		t.Log(eOut.String())
		t.Errorf("expected errors to be reported")
	}
}
