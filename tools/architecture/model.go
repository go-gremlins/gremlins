package main

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

type rule struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Fix         string `json:"fix"`
}
type component struct {
	Allow     []string `json:"allow,omitempty"`
	TestAllow []string `json:"test_allow,omitempty"`
}
type trustBoundary struct {
	Destinations  []string `json:"destinations"`
	Network       bool     `json:"network"`
	ExecuteSource bool     `json:"execute_source"`
}

type model struct {
	ContractChanges map[string][]string      `json:"contract_changes"`
	TrustBoundaries map[string]trustBoundary `json:"trust_boundaries"`
	Version         int                      `json:"version"`
	Module          string                   `json:"module"`
	Rules           []rule                   `json:"rules"`
	Packages        map[string]component     `json:"packages"`
	Contracts       map[string]string        `json:"contracts"`
	Policies        map[string]string        `json:"policies,omitempty"`
}
type compiledModel struct {
	model
	byKind map[string]rule
	byID   map[string]rule
}
type finding struct {
	Rule    string `json:"rule"`
	File    string `json:"file"`
	Subject string `json:"subject"`
}

func validPath(s string) bool {
	return s != "." && fs.ValidPath(s) && !strings.ContainsAny(s, "\\\x00\r\n:") && !strings.HasPrefix(s, "-")
}

func compile(m model) (*compiledModel, error) {
	if m.Version != 1 || m.Module == "" || len(m.Packages) == 0 || len(m.Contracts) == 0 {
		return nil, fmt.Errorf("invalid model version, module, packages, or contracts; fix: restore the version 1 model")
	}
	c := &compiledModel{model: m, byKind: map[string]rule{}, byID: map[string]rule{}}
	if err := c.compileRules(); err != nil {
		return nil, err
	}
	if err := validatePackages(m.Packages); err != nil {
		return nil, err
	}
	if err := validateCanonicalPaths(m); err != nil {
		return nil, err
	}
	if err := validateProposalsModel(m); err != nil {
		return nil, err
	}

	return c, nil
}

func (c *compiledModel) compileRules() error {
	for _, r := range c.Rules {
		supported := slices.Contains([]string{"ownership", "imports", "contracts", "trust"}, r.Kind)
		if !supported || r.ID == "" || strings.TrimSpace(r.Fix) == "" || strings.TrimSpace(r.Description) == "" {
			return fmt.Errorf("invalid rule %q; fix: supply a known kind, stable ID, explanation, and fix", r.ID)
		}
		if _, ok := c.byKind[r.Kind]; ok {
			return fmt.Errorf("duplicate rule kind %q; fix: retain one rule per kind", r.Kind)
		}
		if _, ok := c.byID[r.ID]; ok {
			return fmt.Errorf("duplicate rule ID %q; fix: use unique IDs", r.ID)
		}
		c.byKind[r.Kind] = r
		c.byID[r.ID] = r
	}
	if len(c.byKind) != 4 {
		return fmt.Errorf("missing architecture rules; fix: define ownership, imports, contracts, and trust")
	}

	return nil
}

func validatePackages(packages map[string]component) error {
	for pkg, comp := range packages {
		if !validPath(pkg) {
			return fmt.Errorf("invalid package %q; fix: use a clean relative package path", pkg)
		}
		if err := validateDependencies(pkg, comp.Allow, packages); err != nil {
			return err
		}
		if err := validateDependencies(pkg, comp.TestAllow, packages); err != nil {
			return err
		}
	}

	return nil
}

func validateDependencies(pkg string, deps []string, packages map[string]component) error {
	seen := map[string]bool{}
	for _, dep := range deps {
		if _, ok := packages[dep]; !ok || seen[dep] {
			return fmt.Errorf("invalid dependency %q in %q; fix: declare each target once", dep, pkg)
		}
		seen[dep] = true
	}

	return nil
}

func validateCanonicalPaths(m model) error {
	for _, owners := range []map[string]string{m.Contracts, m.Policies} {
		for name, p := range owners {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("empty canonical name; fix: name each contract and policy")
			}
			if !validPath(p) {
				return fmt.Errorf("invalid contract %q path %q; fix: use a named checkout-relative file", name, p)
			}
		}
	}

	return nil
}

func (c *compiledModel) dependency(file, pkg, dep string, isTest bool) []finding {
	comp, ok := c.Packages[pkg]
	if !ok {
		return []finding{{Rule: c.byKind["ownership"].ID, File: file, Subject: pkg}}
	}
	_, known := c.Packages[dep]
	allowed := slices.Contains(comp.Allow, dep)
	if isTest {
		allowed = allowed || pkg == dep || slices.Contains(comp.TestAllow, dep)
	}
	if known && allowed {
		return nil
	}

	return []finding{{Rule: c.byKind["imports"].ID, File: file, Subject: dep}}
}

func formatFinding(f finding, c *compiledModel) string {
	r := c.byID[f.Rule]

	return fmt.Sprintf("%q file=%q subject=%q: %q Fix: %q", r.ID, f.File, f.Subject, r.Description, r.Fix)
}

func packageOf(file string) string { return path.Dir(file) }

func (m model) canonicalPaths() map[string]string {
	paths := map[string]string{}
	for name, p := range m.Contracts {
		paths["contract:"+name] = p
	}
	for name, p := range m.Policies {
		paths["policy:"+name] = p
	}

	return paths
}
