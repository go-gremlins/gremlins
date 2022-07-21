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
	"github.com/fatih/color"
	"io"
	"sync"
)

var fgRed = color.New(color.FgRed).SprintFunc()

type log struct {
	writer io.Writer
}

var mutex = &sync.Mutex{}
var instance *log

// Init initializes a new logger with the given io.Writer. If no writer is
// provided the logger behaves as NoOp. The initialized instance
// is a singleton.
//
// If one of the logging methods is called, and the logger hasn't been
// initialized yet, a new logger will be initialized with a noOp writer.
func Init(w io.Writer) {
	if w == nil {
		return
	}
	if instance == nil {
		mutex.Lock()
		defer mutex.Unlock()
		if instance == nil {
			instance = &log{writer: w}
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
	instance.writef("%s: %s", fgRed("ERROR"), msg)
}

// Errorln logs an error line.
func Errorln(a any) {
	if instance == nil {
		return
	}
	msg := fmt.Sprintf("%s: %s", fgRed("ERROR"), a)
	instance.writeln(msg)
}

func (l *log) writef(f string, args ...any) {
	_, _ = fmt.Fprintf(instance.writer, f, args...)
}

func (l *log) writeln(a any) {
	_, _ = fmt.Fprintln(instance.writer, a)
}
