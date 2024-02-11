package exclusion

import (
	"fmt"
	"regexp"

	"github.com/spf13/viper"

	"github.com/go-gremlins/gremlins/internal/configuration"
)

type Rules []*regexp.Regexp

func New() (Rules, error) {
	var rules Rules

	// TODO: modify and use configuration package
	// NOTE: configuration.Get can't type cast to []string a value from .gremlins file, because viper.Get(k) returns []interface{}
	flagValues := viper.GetStringSlice(configuration.UnleashExcludeFiles)

	for i, s := range flagValues {
		r, err := regexp.Compile(s)
		if err != nil {
			return nil, fmt.Errorf("error in exclude-files param value #%d: %w", i, err)
		}

		rules = append(rules, r)
	}

	return rules, nil
}

func (r Rules) IsFileExcluded(path string) bool {
	if len(r) == 0 {
		return false
	}

	for _, rule := range r {
		if rule.MatchString(path) {
			return true
		}
	}

	return false
}
