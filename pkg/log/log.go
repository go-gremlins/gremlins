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

package log

import (
	"fmt"
	"io"
	"sync"

	"github.com/fatih/color"
)

var fgRed = color.New(color.FgRed).SprintFunc()

var mutex = &sync.Mutex{}
var instance *log

// Init initializes a new logger with the given out and eOut io.Writer.
// If no out is  provided the logger behaves as NoOp. The initialized instance
// is a singleton.
//
// If one of the logging methods is called, and the logger hasn't been
// initialized yet, a new logger will be initialized with a noOp out.
func Init(out, eOut io.Writer) {
	if out == nil || eOut == nil {
		return
	}
	if instance == nil {
		mutex.Lock()
		defer mutex.Unlock()
		if instance == nil {
			instance = &log{out: out, eOut: eOut}
		}
	}
}

// Reset removes the current log instance.
func Reset() {
	instance = nil
}

// Infof logs an information using format.
func Infof(f string, args ...any) {
	if instance == nil {
		return
	}
	instance.writef(f, args...)
}

// Infoln logs an information line.
func Infoln(a any) {
	if instance == nil {
		return
	}
	instance.writeln(a)
}

// Errorf logs an error using format.
func Errorf(f string, args ...any) {
	if instance == nil {
		return
	}
	msg := fmt.Sprintf(f, args...)
	instance.eWritef("%s: %s", fgRed("ERROR"), msg)
}

// Errorln logs an error line.
func Errorln(a any) {
	if instance == nil {
		return
	}
	msg := fmt.Sprintf("%s: %s", fgRed("ERROR"), a)
	instance.eWriteln(msg)
}

type log struct {
	out  io.Writer
	eOut io.Writer
}

func (l *log) writef(f string, args ...any) {
	_, _ = fmt.Fprintf(l.out, f, args...)
}

func (l *log) writeln(a any) {
	_, _ = fmt.Fprintln(l.out, a)
}

func (l *log) eWritef(f string, args ...any) {
	_, _ = fmt.Fprintf(l.eOut, f, args...)
}

func (l *log) eWriteln(a any) {
	_, _ = fmt.Fprintln(l.eOut, a)
}
