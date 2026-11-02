package main

import (
	"fmt"
	"strings"
)

type baseline struct {
	Version  int       `json:"version"`
	Findings []finding `json:"findings"`
}

func compareFindings(a, b finding) int {
	if n := strings.Compare(a.Rule, b.Rule); n != 0 {
		return n
	}
	if n := strings.Compare(a.File, b.File); n != 0 {
		return n
	}

	return strings.Compare(a.Subject, b.Subject)
}

func findingSet(findings []finding) (map[finding]bool, error) {
	set := map[finding]bool{}
	for _, f := range findings {
		if set[f] || f.Rule == "" || !validPath(f.File) || f.Subject == "" {
			return nil, fmt.Errorf("duplicate or invalid finding identity; fix: retain each valid identity once")
		}
		set[f] = true
	}

	return set, nil
}

func ratchet(current, candidate, target []finding) error {
	actual, err := findingSet(current)
	if err != nil {
		return err
	}
	allowed, err := findingSet(candidate)
	if err != nil {
		return err
	}
	previous, err := findingSet(target)
	if err != nil {
		return err
	}
	for f := range actual {
		if !allowed[f] {
			return fmt.Errorf("new finding %q at %q for %q; fix: repair the violation, never grow the baseline", f.Rule, f.File, f.Subject)
		}
	}
	for f := range allowed {
		if !previous[f] {
			return fmt.Errorf("baseline growth %q at %q for %q; fix: restore the target baseline and repair the violation", f.Rule, f.File, f.Subject)
		}
		if !actual[f] {
			return fmt.Errorf("stale baseline %q at %q for %q; fix: remove this baseline entry", f.Rule, f.File, f.Subject)
		}
	}

	return nil
}

func validateBaseline(b baseline, c *compiledModel) error {
	if b.Version != 1 || b.Findings == nil {
		return fmt.Errorf("invalid baseline; fix: use version 1 and an explicit findings array")
	}
	if _, err := findingSet(b.Findings); err != nil {
		return err
	}
	for _, f := range b.Findings {
		r, ok := c.byID[f.Rule]
		if !ok || r.Kind == "contracts" {
			return fmt.Errorf("unknown or non-baselinable rule %q; fix: repair input or restore the contract", f.Rule)
		}
	}

	return nil
}
