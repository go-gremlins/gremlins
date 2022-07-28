package cmd

import (
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type envEntry struct {
	name  string
	value string
}

func TestConfiguration(t *testing.T) {

	testCases := []struct {
		name         string
		configPaths  []string
		envEntries   []envEntry
		wantedConfig map[string]interface{}
	}{
		{
			name:        "from cfg",
			configPaths: []string{"./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1,tag2,tag3",
			},
		},
		{
			name:        "from cfg multi",
			configPaths: []string{"./testdata/config2", "./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1.2,tag2.2,tag3.2",
			},
		},
		{
			name: "from env",
			envEntries: []envEntry{
				{name: "GREMLINS_UNLEASH_DRY_RUN", value: "true"},
				{name: "GREMLINS_UNLEASH_TAGS", value: "tag1,tag2,tag3"},
			},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": "true",
				"unleash.tags":    "tag1,tag2,tag3",
			},
		},
		{
			name: "from cfg override with env",
			envEntries: []envEntry{
				{name: "GREMLINS_UNLEASH_TAGS", value: "tag1env,tag2env,tag3env"},
			},
			configPaths: []string{"./testdata/config1"},
			wantedConfig: map[string]interface{}{
				"unleash.dry-run": true,
				"unleash.tags":    "tag1env,tag2env,tag3env",
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.envEntries != nil {
				for _, e := range tc.envEntries {
					os.Setenv(e.name, e.value)
				}
			}
			v := getViper(tc.configPaths)

			for key, wanted := range tc.wantedConfig {
				got := v.Get(key)
				if got != wanted {
					t.Errorf(cmp.Diff(got, wanted))
				}
			}

			if tc.envEntries != nil {
				for _, e := range tc.envEntries {
					os.Unsetenv(e.name)
				}
			}
		})
	}
}
