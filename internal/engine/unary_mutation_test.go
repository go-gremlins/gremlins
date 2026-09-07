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

package engine_test

import (
	"context"
	"testing"

	"github.com/go-gremlins/gremlins/internal/configuration"
	"github.com/go-gremlins/gremlins/internal/engine"
	"github.com/go-gremlins/gremlins/internal/mutator"
)

// Go spells `+`, `-`, `&` and `^` the same in prefix and infix position and
// means something different by each. Mutating a prefix `&` as if it were
// bitwise AND turns `&Foo{}` into `|Foo{}`, which does not parse; mutating a
// prefix `^` the same way turns `^u` into `&u`, which no longer has the
// operand's type. Neither is a fault a test suite could have caught, so
// neither is generated.

func TestUnaryOperatorsAreNotMutatedAsBinaryOnes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		fixture string
		// want is the mutator.Type the fixture's single operator must yield,
		// or the zero Type when the fixture must yield no mutant at all.
		want     mutator.Type
		mutating bool
	}{
		{
			name:     "a prefix & is address-of, not bitwise AND",
			fixture:  "testdata/fixtures/unary_and_go",
			mutating: false,
		},
		{
			name:     "a prefix ^ is complement, not XOR",
			fixture:  "testdata/fixtures/unary_xor_go",
			mutating: false,
		},
		{
			name:     "an infix & is still INVERT_BITWISE",
			fixture:  "testdata/fixtures/and_go",
			want:     mutator.InvertBitwise,
			mutating: true,
		},
		{
			name:     "an infix ^ is still INVERT_BITWISE",
			fixture:  "testdata/fixtures/xor_go",
			want:     mutator.InvertBitwise,
			mutating: true,
		},
		{
			name:     "a prefix - is still INVERT_NEGATIVES",
			fixture:  "testdata/fixtures/negative_sub_go",
			want:     mutator.InvertNegatives,
			mutating: true,
		},
		{
			name:     "an infix + is still ARITHMETIC_BASE",
			fixture:  "testdata/fixtures/add_go",
			want:     mutator.ArithmeticBase,
			mutating: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			viperSet(map[string]any{configuration.UnleashDryRunKey: true})
			defer viperReset()

			mapFS, mod, c := loadFixture(tc.fixture, ".")
			defer c()

			mut := engine.New(mod, engine.CodeData{}, newJobDealerStub(t), engine.WithDirFs(mapFS))
			got := mut.Run(context.Background()).Mutants

			if !tc.mutating {
				if len(got) != 0 {
					t.Fatalf("expected no mutant, got %d: %s", len(got), describe(got))
				}

				return
			}

			for _, g := range got {
				if g.Type() == tc.want {
					return
				}
			}

			t.Fatalf("expected a %s mutant, got %s", tc.want, describe(got))
		})
	}
}
