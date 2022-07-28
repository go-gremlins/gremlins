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

/*
Gremlins is a mutation testing tool for Go.
It has been made to work well on smallish Go modules, for example microservices, on which it helps validate the tests, aids the TDD process and can be used as a CI quality gate.
As of now, Gremlins doesn't work very well on very big Go modules, mainly because a run can take hours to complete.

Usage

To execute a mutation test run, from the root of a Go module execute:

	  $ gremlins unleash

If the Go test run needs build tags, they can be passed along:

   $ gremlins unleash --tags "tag1,tag2"

To perform the analysis without actually running the tests:

  $ gremlins unleash --dry-run


Gremlins will report each mutation as:
 - RUNNABLE: In dry-run mode, a mutation that can be tested.
 - NOT COVERED: A mutation not covered by tests; it will not be tested.
 - KILLED: The mutation has been caught by the test suite.
 - LIVED: The mutation hasn't been caught by the test suite.
 - TIMED OUT: The tests timed out while testing the mutation: the mutation actually made the tests fail, but not explicitly.
 - NOT VIABLE: The mutation makes the build fail.

Configuration

Gremlins uses Viper (https://github.com/spf13/viper) for the configuration.

In particular, the options can be passed in the following ways

 - specific command flags
 - environment variables
 - configuration file

in which each item takes precedence over the following in the list.
The environment variables must be set with the following syntax:

  GREMLINS_<COMMAND NAME>_<FLAG NAME>

in which every dash in the option name  must be replaced with an underscore.

Example:

  $ GREMLINS_UNLEASH_DRY_RUN=true gremlins unleash


The configuration must be named
 .gremlins.yaml
and must be in the following format:

 unleash:
   dry-run: false
   tags: ...

and can be placed in one of the following folder (in order)

 - the current folder
 - /etc/gremlins
 - $HOME/.gremlins
*/
package gremlins
