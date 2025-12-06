//go:build unix

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

package engine

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup configures the command to run in a new process group.
// This ensures that child processes (e.g., test binaries spawned by go test)
// can be cleaned up together when the parent is killed.
func setupProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup kills the process and all its children by sending
// SIGKILL to the entire process group. This prevents orphaned processes
// from accumulating and exhausting system resources.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Negative PID kills the entire process group
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
