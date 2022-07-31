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

package cmd

import (
	"testing"
)

func TestGremlins(t *testing.T) {
	c, err := newRootCmd("1.2.3")
	if err != nil {
		t.Fatal("newRootCmd should not fail")
	}
	_ = c.execute()
	cmd := c.cmd

	if cmd.Version != "1.2.3" {
		t.Errorf("expected %q, got %q", "1.2.3", cmd.Version)
	}

	cfgFile := cmd.Flag("config")
	if cfgFile == nil {
		t.Fatal("expected to have a config flag")
	}
	if cfgFile.Value.Type() != "string" {
		t.Errorf("expected value type to be 'string', got %v", cfgFile.Value.Type())
	}
	if cfgFile.DefValue != "" {
		t.Errorf("expected default value to be empty, got %v", cfgFile.DefValue)
	}
}

func TestExecute(t *testing.T) {
	t.Run("should not fail", func(t *testing.T) {
		err := Execute("1.2.3")
		if err != nil {
			t.Errorf("execute should not fail")
		}
	})

	t.Run("should fail if version is not set", func(t *testing.T) {
		err := Execute("")
		if err == nil {
			t.Errorf("expected failure")
		}

	})
}
