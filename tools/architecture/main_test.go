package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const dependencyRule = "DEP"
const appPackage = "app"
const corePackage = "core"
const appFile = "app/a.go"
const changeSummary = "change"
const contractKind = "contract"
const cliContract = "cli"

func testModel(t *testing.T) *compiledModel {
	t.Helper()
	m := model{
		ContractChanges: map[string][]string{cliContract: {compatibleChange}},
		TrustBoundaries: localTrust(),
		Version:         1,
		Module:          "example.org/app",
		Rules: []rule{
			{ID: "OWN", Kind: "ownership", Description: "owned", Fix: "declare package"},
			{ID: dependencyRule, Kind: "imports", Description: "boundary", Fix: "remove dependency"},
			{ID: "CON", Kind: "contracts", Description: contractKind, Fix: "restore contract"},
			{ID: "TRUST", Kind: "trust", Description: "trusted checks", Fix: "restore local boundary"},
		},
		Packages:  map[string]component{appPackage: {Allow: []string{corePackage}}, corePackage: {}},
		Contracts: map[string]string{cliContract: appFile},
	}
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func writeFile(t *testing.T, root, name, contents string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScanBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, file, source, want string }{
		{"allowed", appFile, "package app; import _ `example.org/app/core`", ""},
		{"forbidden alias", "core/a.go", "package core; import alias `example.org/app/app`", dependencyRule},
		{
			"platform tag",
			"core/a_windows.go",
			"//go:build windows\n\npackage core; import _ `example.org/app/app`",
			dependencyRule,
		},
		{"test scope", "core/a_test.go", "package core_test; import _ `example.org/app/app`", dependencyRule},
		{"own external test", "core/a_test.go", "package core_test; import _ `example.org/app/core`", ""},
		{"unowned", "new/a.go", "package new", "OWN"},
		{"fixture", "testdata/new/a.go", "not Go syntax", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, appFile, "package app")
			writeFile(t, root, tc.file, tc.source)
			findings, err := scan(root, testModel(t))
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" && len(findings) != 0 {
				t.Fatalf("unexpected: %v", findings)
			}
			if tc.want != "" && (len(findings) != 1 || findings[0].Rule != tc.want) {
				t.Fatalf("want %s, got %v", tc.want, findings)
			}
		})
	}
}

func TestBaselineRatchet(t *testing.T) {
	a := finding{Rule: dependencyRule, File: "core/a.go", Subject: appPackage}
	b := finding{Rule: dependencyRule, File: "core/b.go", Subject: appPackage}
	for _, tc := range []struct {
		name                       string
		current, candidate, target []finding
		bad                        bool
	}{
		{"empty", nil, nil, nil, false}, {"retained", []finding{a}, []finding{a}, []finding{a}, false},
		{"shrunk", nil, nil, []finding{a}, false}, {"new violation", []finding{a}, nil, nil, true},
		{
			"grown",
			[]finding{a},
			[]finding{a},
			nil,
			true,
		}, {
			"equal count substitution",
			[]finding{b},
			[]finding{b},
			[]finding{a},
			true,
		},
		{"stale", nil, []finding{a}, []finding{a}, true}, {"duplicate", []finding{a}, []finding{a, a}, []finding{a}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ratchet(tc.current, tc.candidate, tc.target)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestDesignUsesCompiledRules(t *testing.T) {
	for _, tc := range []struct {
		name, pkg, dep string
		bad            bool
	}{
		{
			"allowed",
			appPackage,
			corePackage,
			false,
		}, {
			"forbidden",
			corePackage,
			appPackage,
			true,
		}, {
			"unknown source",
			"other",
			appPackage,
			true,
		}, {
			"unknown target",
			appPackage,
			"other",
			true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := design{
				Version:         1,
				Issue:           76,
				Summary:         "planned change",
				TrustChanges:    localTrust(),
				TrustBoundaries: "local only",
				Changes:         []change{{Package: tc.pkg, Imports: []string{tc.dep}}},
			}
			err := checkDesign(testModel(t), d)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestStrictJSON(t *testing.T) {
	for _, data := range []string{
		`{"version":1,"version":2}`,
		`{"version":1,"unknown":0}`,
		`{"version":1} {}`,
		`null`,
		`{"version":1,"findings":null}`,
	} {
		var b baseline
		if err := decode([]byte(data), &b); err == nil {
			t.Errorf("accepted %s", data)
		}
	}
	var b baseline
	if err := decode([]byte(`{"version":1,"findings":[]}`), &b); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidModels(t *testing.T) {
	cases := []func(*model){
		func(m *model) { m.Version = 2 },
		func(m *model) { m.Rules[0].Fix = "" },
		func(m *model) { m.Rules[1].ID = m.Rules[0].ID },
		func(m *model) { m.Rules[1].Kind = "invalid" },
		func(m *model) { m.Packages["../escape"] = component{} },
		func(m *model) { m.Contracts[cliContract] = "/tmp/outside" },
		func(m *model) { m.Packages[appPackage] = component{Allow: []string{"missing"}} },
	}
	for i, mutate := range cases {
		m := testModel(t).model
		mutate(&m)
		if _, err := compile(m); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestScanFailsClosed(t *testing.T) {
	for _, name := range []string{"symlink", "parse", "nested module"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, appFile, "package app")
			switch name {
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "linked")); err != nil {
					t.Skip(err)
				}
			case "parse":
				writeFile(t, root, "core/a.go", "garbage")
			case "nested module":
				writeFile(t, root, "nested/go.mod", "module nested")
			default:
				t.Fatalf("unknown scan case %q", name)
			}
			if _, err := scan(root, testModel(t)); err == nil {
				t.Fatal("scan succeeded")
			}
		})
	}
}

func TestSafePathsAndDiagnostics(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "regular", "ok")
	for _, name := range []string{"../outside", "/tmp/outside", "a/../regular"} {
		if _, err := readLocal(root, name); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
	if err := os.Symlink(filepath.Join(root, "regular"), filepath.Join(root, "link")); err == nil {
		if _, err := readLocal(root, "link"); err == nil {
			t.Fatal("followed link")
		}
	}
	msg := formatFinding(finding{Rule: dependencyRule, File: "core/\n::error::.go", Subject: appPackage}, testModel(t))
	if strings.Contains(msg, "\n") {
		t.Fatalf("unescaped %q", msg)
	}
}

func TestDesignConflictsAndContracts(t *testing.T) {
	for _, tc := range []struct{ conflicts, contracts []string }{
		{conflicts: []string{"unresolved"}},
		{contracts: []string{unknownName}},
	} {
		d := design{
			Version:         1,
			Issue:           76,
			Summary:         changeSummary,
			TrustChanges:    localTrust(),
			TrustBoundaries: localBoundary,
			Conflicts:       tc.conflicts,
			Contracts:       tc.contracts,
		}
		if err := checkDesign(testModel(t), d); err == nil {
			t.Fatal("accepted conflicting design")
		}
	}
}

func TestModelFixtureRoundTrip(t *testing.T) {
	data, err := json.Marshal(testModel(t).model)
	if err != nil {
		t.Fatal(err)
	}
	var m model
	if err := decode(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, err := compile(m); err != nil {
		t.Fatal(err)
	}
}

func TestScanAllowsInternalDocumentationLink(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, appFile, "package app")
	writeFile(t, root, "docs/source.md", "documentation")
	if err := os.Symlink("source.md", filepath.Join(root, "docs", "link.md")); err != nil {
		t.Skip(err)
	}
	if _, err := scan(root, testModel(t)); err != nil {
		t.Fatal(err)
	}
}

func TestRunEndToEnd(t *testing.T) {
	for _, tc := range []struct {
		name, proposed, base string
		edit                 func(*testing.T, string)
		bad                  bool
	}{
		{name: "source", base: "HEAD"},
		{name: "design", proposed: designFile},
		{name: "first adoption"},
		{name: "missing history", base: "unavailable", bad: true},
		{
			name: "invalid model",
			edit: func(t *testing.T, r string) { writeFile(t, r, modelFile, `{"version":2}`) },
			bad:  true,
		},
		{name: "malformed model", edit: func(t *testing.T, r string) { writeFile(t, r, modelFile, `{`) }, bad: true},
		{name: "missing model", edit: func(t *testing.T, r string) {
			if err := os.Remove(filepath.Join(r, modelFile)); err != nil {
				t.Fatal(err)
			}
		}, bad: true},
		{name: "malformed baseline", edit: func(t *testing.T, r string) { writeFile(t, r, baselineFile, `{`) }, bad: true},
		{
			name: "invalid baseline",
			edit: func(t *testing.T, r string) { writeFile(t, r, baselineFile, `{"version":2,"findings":[]}`) },
			bad:  true,
		},
		{name: "new violation", edit: func(t *testing.T, r string) {
			writeFile(t, r, "core/a.go", "package core; import _ `example.org/app/app`")
		}, bad: true},
		{name: "broken source", edit: func(t *testing.T, r string) { writeFile(t, r, "core/a.go", "not Go") }, bad: true},
		{
			name:     "invalid design",
			proposed: designFile,
			edit:     func(t *testing.T, r string) { writeFile(t, r, designFile, `{"version":2}`) },
			bad:      true,
		},
		{
			name:     "malformed design",
			proposed: designFile,
			edit:     func(t *testing.T, r string) { writeFile(t, r, designFile, `{`) },
			bad:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := gitFixture(t)
			m := testModel(t).model
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, modelFile, string(data))
			writeFile(t, root, baselineFile, `{"version":1,"findings":[]}`)
			writeFile(t, root, designFile, `{
  "version": 1,
  "issue": 76,
  "summary": "planned",
  "trust_boundaries": "local",
  "trust_changes": {
    "checks": {
      "destinations": [
        "local"
      ]
    }
  },
  "contract_changes": {
    "cli": "compatible"
  },
  "contracts": [
    "cli"
  ],
  "changes": [
    {
      "package": "app",
      "imports": [
        "core"
      ]
    }
  ]
}`)
			writeFile(t, root, appFile, "package app")
			if tc.edit != nil {
				tc.edit(t, root)
			}
			t.Chdir(root)
			if err := run(tc.base, tc.proposed); (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestBaselineValidation(t *testing.T) {
	for _, b := range []baseline{
		{Version: 2, Findings: []finding{}},
		{Version: 1, Findings: []finding{{Rule: "UNKNOWN", File: appFile, Subject: "x"}}},
		{Version: 1, Findings: []finding{{Rule: "CON", File: appFile, Subject: cliContract}}},
	} {
		if err := validateBaseline(b, testModel(t)); err == nil {
			t.Fatal("invalid baseline accepted")
		}
	}
}

func TestInputRegularFileAndNestedDuplicate(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "directory"), 0750); err != nil {
		t.Fatal(err)
	}
	if _, err := readLocal(root, "directory"); err == nil {
		t.Fatal("read directory")
	}
	var d design
	if err := decode([]byte(`{"changes":[{"package":"app","package":"core"}]}`), &d); err == nil {
		t.Fatal("nested duplicate accepted")
	}
}

func TestModelMessagesAreEscaped(t *testing.T) {
	c := testModel(t)
	r := c.byID[dependencyRule]
	r.Description = "bad\n::error::"
	r.Fix = "fix\nnext"
	c.byID[dependencyRule] = r
	if strings.Contains(formatFinding(finding{Rule: dependencyRule, File: appFile, Subject: corePackage}, c), "\n") {
		t.Fatal("model text injected a log line")
	}
}

func TestEmptyCanonicalNamesRejected(t *testing.T) {
	for _, contract := range []bool{true, false} {
		m := testModel(t).model
		if contract {
			m.Contracts[""] = appFile
			m.ContractChanges[""] = []string{compatibleChange}
		} else {
			m.Policies = map[string]string{"": appFile}
		}
		if _, err := compile(m); err == nil {
			t.Fatal("empty canonical name accepted")
		}
	}
}

func TestScanResolvesCheckoutParentAliases(t *testing.T) {
	parent := t.TempDir()
	realParent := filepath.Join(parent, "real")
	if err := os.MkdirAll(filepath.Join(realParent, "checkout"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realParent, filepath.Join(parent, "alias")); err != nil {
		t.Skip(err)
	}
	root := filepath.Join(parent, "alias", "checkout")
	writeFile(t, root, appFile, "package app")
	writeFile(t, root, "docs/source.md", "documentation")
	if err := os.Symlink("source.md", filepath.Join(root, "docs/link.md")); err != nil {
		t.Skip(err)
	}
	if findings, err := scan(root, testModel(t)); err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v error=%v", findings, err)
	}
}
