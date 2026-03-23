/*
 * Copyright 2024 The Gremlins Authors
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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestExtractSnippet(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		content      []byte
		targetLine   int
		contextLines int
		want         []byte
	}{
		"should_clamp_start_to_zero_when_target_line_is_near_beginning": {
			content:      []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"),
			targetLine:   1,
			contextLines: 3,
			want:         []byte("line1\nline2\nline3\nline4"),
		},
		"should_extract_lines_around_target_when_target_is_in_the_middle": {
			content:      []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"),
			targetLine:   5,
			contextLines: 2,
			want:         []byte("line3\nline4\nline5\nline6\nline7"),
		},
		"should_clamp_end_to_content_length_when_target_line_is_near_end": {
			content:      []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"),
			targetLine:   9,
			contextLines: 3,
			want:         []byte("line6\nline7\nline8\nline9\nline10"),
		},
		"should_return_all_content_when_context_is_larger_than_file": {
			content:      []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"),
			targetLine:   5,
			contextLines: 20,
			want:         []byte("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"),
		},
		"should_handle_trailing_newline_in_content": {
			content:      []byte("line1\nline2\nline3\n"),
			targetLine:   2,
			contextLines: 1,
			want:         []byte("line1\nline2\nline3"),
		},
		"should_return_only_target_line_when_context_is_zero": {
			content:      []byte("line1\nline2\nline3"),
			targetLine:   2,
			contextLines: 0,
			want:         []byte("line2"),
		},
		"should_return_empty_when_content_is_empty": {
			content:      []byte(""),
			targetLine:   1,
			contextLines: 3,
			want:         []byte(""),
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if diff := cmp.Diff(string(tc.want), string(extractSnippet(tc.content, tc.targetLine, tc.contextLines))); diff != "" {
				t.Errorf("extractSnippet() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
