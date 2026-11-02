package main

import (
	"strings"
	"testing"
)

const localBoundary = "local"
const compatibleChange = "compatible"
const unknownName = "unknown"

func localTrust() map[string]trustBoundary {
	return map[string]trustBoundary{"checks": {Destinations: []string{localBoundary}}}
}

func TestDesignContractCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		bad        bool
	}{
		{"preserved", compatibleChange, false}, {"breaking", "breaking", true}, {unknownName, "unspecified", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := design{
				Version:         1,
				Issue:           76,
				Summary:         changeSummary,
				TrustBoundaries: localBoundary,
				TrustChanges:    localTrust(),
				Contracts:       []string{cliContract},
				ContractChanges: map[string]string{cliContract: tc.kind},
			}
			err := checkDesign(testModel(t), d)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if tc.bad && !strings.Contains(err.Error(), "CON") {
				t.Fatalf("no compiled rule diagnostic: %v", err)
			}
		})
	}
}

func TestDesignTrustCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name     string
		proposal trustBoundary
		bad      bool
	}{
		{localBoundary, trustBoundary{Destinations: []string{localBoundary}}, false},
		{"external upload", trustBoundary{Destinations: []string{"new-service"}}, true},
		{"network", trustBoundary{Destinations: []string{localBoundary}, Network: true}, true},
		{"source execution", trustBoundary{Destinations: []string{localBoundary}, ExecuteSource: true}, true},
		{"missing destinations", trustBoundary{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := design{
				Version:         1,
				Issue:           76,
				Summary:         changeSummary,
				TrustBoundaries: localBoundary,
				TrustChanges:    map[string]trustBoundary{"checks": tc.proposal},
			}
			err := checkDesign(testModel(t), d)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if tc.bad && !strings.Contains(err.Error(), "TRUST") {
				t.Fatalf("no compiled rule diagnostic: %v", err)
			}
		})
	}
}

func TestDesignRequiresModeledAssertions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*design)
	}{
		{"missing trust", func(d *design) { d.TrustChanges = nil }},
		{"unknown boundary", func(d *design) {
			d.TrustChanges = map[string]trustBoundary{unknownName: {Destinations: []string{localBoundary}}}
		}},
		{"missing contract kind", func(d *design) { d.Contracts = []string{cliContract} }},
		{"undeclared contract", func(d *design) { d.ContractChanges = map[string]string{cliContract: compatibleChange} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := design{
				Version:         1,
				Issue:           76,
				Summary:         changeSummary,
				TrustBoundaries: localBoundary,
				TrustChanges:    localTrust(),
			}
			tc.change(&d)
			if err := checkDesign(testModel(t), d); err == nil {
				t.Fatal("incomplete assertions accepted")
			}
		})
	}
}

func TestInvalidProposalModels(t *testing.T) {
	for _, mutate := range []func(*model){
		func(m *model) { delete(m.ContractChanges, cliContract) },
		func(m *model) { m.ContractChanges[unknownName] = []string{compatibleChange} },
		func(m *model) { m.ContractChanges[cliContract] = []string{unknownName} },
		func(m *model) { m.TrustBoundaries = nil },
		func(m *model) { m.TrustBoundaries["checks"] = trustBoundary{} },
	} {
		m := testModel(t).model
		mutate(&m)
		if _, err := compile(m); err == nil {
			t.Fatal("invalid proposal policy compiled")
		}
	}
}

func TestDesignTestDependencyScope(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		imports, testImports []string
		bad                  bool
	}{
		{"production rejects test-only", []string{appPackage}, nil, true},
		{"test-only allowed", nil, []string{appPackage}, false},
		{"external test self import", nil, []string{corePackage}, false},
		{"test rejects unknown", nil, []string{unknownName}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testModel(t)
			c.Packages[corePackage] = component{TestAllow: []string{appPackage}}
			d := design{
				Version:         1,
				Issue:           76,
				Summary:         changeSummary,
				TrustBoundaries: localBoundary,
				TrustChanges:    localTrust(),
				Changes:         []change{{Package: corePackage, Imports: tc.imports, TestImports: tc.testImports}},
			}
			if err := checkDesign(c, d); (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
		})
	}
}
