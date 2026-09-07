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

// Gremlins is a mutation testing tool for Go.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/fatih/color"

	"github.com/go-gremlins/gremlins/cmd"
	"github.com/go-gremlins/gremlins/internal/execution"
	"github.com/go-gremlins/gremlins/internal/log"
)

var version = "dev"

func main() {
	var exitErr *execution.ExitError
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()
	log.Init(color.Output, color.Error)
	ctx := ctxDoneOnSignal()
	err := cmd.Execute(ctx, buildVersion(version))
	if err != nil {
		log.Errorln(err)
		exitCode = 1
	}
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}
}

func ctxDoneOnSignal() context.Context {
	// Buffer of 2 lets a follow-up signal be queued (e.g. CI runner sending
	// SIGTERM then SIGKILL is masked, but Ctrl-C twice from a TTY isn't).
	// We never close `done`: closing while `os/signal` may still write to it
	// causes "panic: send on closed channel" from signal.process. Instead,
	// we call signal.Stop to detach the runtime sender before we drop our
	// reference to the channel.
	done := make(chan os.Signal, 2)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-done
		cancel()
		// A second signal forces immediate exit. The first one triggered
		// the graceful-shutdown path via the cancelled ctx; if the user
		// hits Ctrl-C again they want out _now_.
		go func() {
			<-done
			signal.Stop(done)
			os.Exit(130) // 128 + SIGINT
		}()
	}()

	return ctx
}

func buildVersion(version string) string {
	return fmt.Sprintf("%s %s/%s", version, runtime.GOOS, runtime.GOARCH)
}
