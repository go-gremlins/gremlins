package main

import "fmt"

type change struct {
	Package     string   `json:"package"`
	Imports     []string `json:"imports,omitempty"`
	TestImports []string `json:"test_imports,omitempty"`
}
type design struct {
	ContractChanges map[string]string        `json:"contract_changes,omitempty"`
	TrustChanges    map[string]trustBoundary `json:"trust_changes"`
	Version         int                      `json:"version"`
	Issue           int                      `json:"issue"`
	Summary         string                   `json:"summary"`
	Changes         []change                 `json:"changes,omitempty"`
	Contracts       []string                 `json:"contracts,omitempty"`
	TrustBoundaries string                   `json:"trust_boundaries"`
	Conflicts       []string                 `json:"conflicts,omitempty"`
	Policies        []string                 `json:"policies,omitempty"`
}

func checkDesign(c *compiledModel, d design) error {
	if d.Version != 1 || d.Issue < 1 || d.Summary == "" || d.TrustBoundaries == "" {
		return fmt.Errorf("incomplete design; fix: supply version, issue, summary, and trust-boundary review")
	}
	if len(d.Conflicts) > 0 {
		return fmt.Errorf("unresolved design conflicts; fix: resolve them before coding")
	}
	for _, contract := range d.Contracts {
		if _, ok := c.Contracts[contract]; !ok {
			return fmt.Errorf("unknown contract %q; fix: use canonical model contract names", contract)
		}
	}
	for _, policy := range d.Policies {
		if _, ok := c.Policies[policy]; !ok {
			return fmt.Errorf("unknown policy %q; fix: declare a canonical policy name", policy)
		}
	}
	if err := checkPlannedPackages(c, d.Changes); err != nil {
		return err
	}

	return checkProposals(c, d)
}

func checkPlannedPackages(c *compiledModel, changes []change) error {
	seen := map[string]bool{}
	for _, planned := range changes {
		if _, ok := c.Packages[planned.Package]; !ok || seen[planned.Package] {
			return fmt.Errorf("invalid design package %q; fix: declare each owned package once", planned.Package)
		}
		seen[planned.Package] = true
		if err := checkPlannedImports(c, planned); err != nil {
			return err
		}
	}

	return nil
}

func checkPlannedImports(c *compiledModel, planned change) error {
	for _, isTest := range []bool{false, true} {
		imports := planned.Imports
		if isTest {
			imports = planned.TestImports
		}
		for _, dep := range imports {
			findings := c.dependency(planned.Package, planned.Package, dep, isTest)
			if len(findings) > 0 {
				return fmt.Errorf("design: %s", formatFinding(findings[0], c))
			}
		}
	}

	return nil
}
