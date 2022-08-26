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

package internal

// OutputResult is the data structure for the Gremlins file output format.
type OutputResult struct {
	GoModule          string       `json:"go_module"`
	Files             []OutputFile `json:"files"`
	TestEfficacy      float64      `json:"test_efficacy"`
	MutationsCoverage float64      `json:"mutations_coverage"`
	MutantsTotal      int          `json:"mutants_total"`
	MutantsKilled     int          `json:"mutants_killed"`
	MutantsLived      int          `json:"mutants_lived"`
	MutantsNotViable  int          `json:"mutants_not_viable"`
	MutantsNotCovered int          `json:"mutants_not_covered"`
	ElapsedTime       float64      `json:"elapsed_time"`
}

// OutputFile represents a single file in the OutputResult data structure.
type OutputFile struct {
	Filename  string     `json:"file_name"`
	Mutations []Mutation `json:"mutations"`
}

// Mutation represents a single mutation in the OutputResult data structure.
type Mutation struct {
	Type   string `json:"type"`
	Status string `json:"status"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}
