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

package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/fatih/color"

	"github.com/go-gremlins/gremlins/cmd"
	"github.com/go-gremlins/gremlins/log"
)

var (
	version = "dev"
	date    = ""
	builtBy = ""
)

func main() {
	var exitCode int
	defer func() {
		os.Exit(exitCode)
	}()
	log.Init(color.Output, color.Error)
	err := cmd.Execute(buildVersion(version, date, builtBy))
	if err != nil {
		log.Errorln(err)
		exitCode = 1
	}
}

func buildVersion(version, date, builtBy string) string {
	result := version
	if date != "" {
		result = fmt.Sprintf("%s\n\tbuilt at %s", result, date)
	}
	if builtBy != "" {
		result = fmt.Sprintf("%s by %s", result, builtBy)
	}
	result = fmt.Sprintf("%s\n\tGOOS: %s\n\tGOARCH: %s", result, runtime.GOOS, runtime.GOARCH)

	return result
}
