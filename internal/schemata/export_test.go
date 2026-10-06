/*
 * Copyright 2026 The Gremlins Authors
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

package schemata

import (
	"errors"
	"go/token"
)

// ConstMutation exposes constMutation to the external tests, which hold it
// against the engine's table.
var ConstMutation = constMutation

// RewriterFactory exposes rewriterFactory, for tests that force a rewrite.
type RewriterFactory = rewriterFactory

// RewritePackageWith exposes rewritePackage with an injected rewriter.
var RewritePackageWith = rewritePackage

// BinaryNames exposes binaryNames.
var BinaryNames = binaryNames

// AssignMutation exposes assignMutation to the external tests, which hold
// it against the engine's table.
var AssignMutation = assignMutation

// BreakDupMutant returns err, a rewriter's request to duplicate a site's
// function, with mutant id's duplicate made not to type-check: its operator
// becomes &&, which no constant operand of an arithmetic site accepts. Any
// other err is returned as it is.
func BreakDupMutant(err error, id int) error {
	var d *dupError
	if errors.As(err, &d) {
		d.mutated[id] = token.LAND
	}

	return err
}

// Mirrors of the helper constraints, which the external tests hold against
// the constraints the generated helpers declare.
var (
	NumberConstraint  = numberConstraint
	IntegerConstraint = integerConstraint
	OrderedConstraint = orderedConstraint
	StringConstraint  = stringConstraint
)

// ErrNewlineChanged exposes errNewlineChanged.
var ErrNewlineChanged = errNewlineChanged

// BuildFunc exposes buildFunc, the seam a test builds a package through.
type BuildFunc = buildFunc

// BuildTest exposes buildTest, the build BuildAll uses.
var BuildTest BuildFunc = buildTest

// BuildAllWith exposes buildAll, BuildAll with an injected build.
var BuildAllWith = buildAll

// PrepareWith exposes prepare, Prepare with an injected build.
var PrepareWith = prepare

// OutOfMemoryMarkers exposes outOfMemoryMarkers.
var OutOfMemoryMarkers = outOfMemoryMarkers

// ShowsOutOfMemory exposes showsOutOfMemory.
var ShowsOutOfMemory = showsOutOfMemory
