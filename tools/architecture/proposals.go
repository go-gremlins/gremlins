package main

import (
	"fmt"
	"slices"
)

func validateProposalsModel(m model) error {
	for name := range m.Contracts {
		kinds := m.ContractChanges[name]
		if len(kinds) == 0 {
			return fmt.Errorf("missing contract policy %q; fix: declare allowed change kinds", name)
		}
		for _, kind := range kinds {
			if !slices.Contains([]string{"compatible", "breaking"}, kind) {
				return fmt.Errorf("invalid contract change kind %q; fix: use compatible or breaking", kind)
			}
		}
	}
	for name := range m.ContractChanges {
		if _, ok := m.Contracts[name]; !ok {
			return fmt.Errorf("unknown contract policy %q; fix: declare the contract first", name)
		}
	}
	if len(m.TrustBoundaries) == 0 {
		return fmt.Errorf("missing trust boundaries; fix: model the development-check data and execution boundary")
	}
	for name, b := range m.TrustBoundaries {
		if name == "" || len(b.Destinations) == 0 {
			return fmt.Errorf("invalid trust boundary %q; fix: name the boundary and approved destinations", name)
		}
	}

	return nil
}

func proposalError(c *compiledModel, kind, subject string) error {
	return fmt.Errorf("design: %s", formatFinding(finding{Rule: c.byKind[kind].ID, File: designFile, Subject: subject}, c))
}

func checkProposals(c *compiledModel, d design) error {
	for _, name := range d.Contracts {
		if !slices.Contains(c.ContractChanges[name], d.ContractChanges[name]) {
			return proposalError(c, "contracts", name)
		}
	}
	for name := range d.ContractChanges {
		if !slices.Contains(d.Contracts, name) {
			return proposalError(c, "contracts", name)
		}
	}
	for name := range c.TrustBoundaries {
		if _, ok := d.TrustChanges[name]; !ok {
			return proposalError(c, "trust", name)
		}
	}
	for name, planned := range d.TrustChanges {
		allowed, ok := c.TrustBoundaries[name]
		if !ok || len(planned.Destinations) == 0 || (planned.Network && !allowed.Network) || (planned.ExecuteSource && !allowed.ExecuteSource) {
			return proposalError(c, "trust", name)
		}
		for _, destination := range planned.Destinations {
			if !slices.Contains(allowed.Destinations, destination) {
				return proposalError(c, "trust", name)
			}
		}
	}

	return nil
}
