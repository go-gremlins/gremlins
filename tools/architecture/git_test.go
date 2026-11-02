package main

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Test"}, {"config", "user.email", "test@example.invalid"}} {
		if _, err := gitRead(root, args...); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, root, "README.md", "base\n")
	commitFixture(t, root)

	return root
}
func commitFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"add", "--all"}, {"-c", "core.hooksPath=/dev/null", "commit", "-qm", "fixture"}} {
		if _, err := gitRead(root, args...); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGitBaselineHistory(t *testing.T) {
	root := gitFixture(t)
	b, revision, err := targetBaseline(root, "HEAD")
	if err != nil || b.Version != 1 || len(b.Findings) != 0 || revision == "" {
		t.Fatalf("first adoption: %v %q %v", b, revision, err)
	}
	writeFile(t, root, baselineFile, `{"version":1,"findings":[{"rule":"DEP","file":"core/a.go","subject":"app"}]}`)
	commitFixture(t, root)
	b, _, err = targetBaseline(root, "HEAD")
	if err != nil || len(b.Findings) != 1 {
		t.Fatalf("baseline: %v %v", b, err)
	}
	for _, ref := range []string{"missing-history", "--help", "HEAD:README.md"} {
		if _, _, err := targetBaseline(root, ref); err == nil {
			t.Errorf("accepted ref %q", ref)
		}
	}
}

func TestChangedDesignGate(t *testing.T) {
	root := gitFixture(t)
	c := testModel(t)
	writeFile(t, root, "core/a.go", "package core")
	if err := checkChangedDesign(root, "HEAD", c); err == nil {
		t.Fatal("missing design accepted")
	}
	d := design{Version: 1, Issue: 76, Summary: changeSummary, TrustChanges: localTrust(), TrustBoundaries: localBoundary, Changes: []change{{Package: appPackage, Imports: []string{corePackage}}}}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, designFile, string(data))
	if err := checkChangedDesign(root, "HEAD", c); err == nil {
		t.Fatal("uncovered package accepted")
	}
	d.Changes = []change{{Package: corePackage, Imports: []string{}}}
	data, err = json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, designFile, string(data))
	if err := checkChangedDesign(root, "HEAD", c); err != nil {
		t.Fatal(err)
	}
}

func TestMalformedTargetBaseline(t *testing.T) {
	root := gitFixture(t)
	writeFile(t, root, baselineFile, `{"version":1,"version":2,"findings":[]}`)
	commitFixture(t, root)
	if _, _, err := targetBaseline(root, "HEAD"); err == nil {
		t.Fatal("accepted duplicate target policy")
	}
}

func TestGitDiagnosticDoesNotLeakSubprocessOutput(t *testing.T) {
	root := gitFixture(t)
	_, err := gitRead(root, "show", "nonexistent:private-file")
	if err == nil || strings.Contains(err.Error(), "private-file") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestContractAndPolicyChangesNeedDesign(t *testing.T) {
	for _, tc := range []struct{ name, path string }{
		{contractKind, appFile}, {"policy", ".architecture/model.json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := gitFixture(t)
			c := testModel(t)
			c.Policies = map[string]string{"model": modelFile}
			// A non-Go canonical contract is subject to the same design step.
			if tc.name == contractKind {
				c.Contracts[cliContract] = "schema.json"
				tc.path = "schema.json"
			}
			writeFile(t, root, tc.path, "changed")
			if err := checkChangedDesign(root, "HEAD", c); err == nil {
				t.Fatal("modeled change passed without a design")
			}
			writeFile(t, root, designFile, `{"version":1,"issue":76,"summary":"planned","trust_boundaries":"reviewed"}`)
			if err := checkChangedDesign(root, "HEAD", c); err == nil {
				t.Fatal("undeclared modeled change passed")
			}
			d := design{Version: 1, Issue: 76, Summary: changeSummary, TrustChanges: localTrust(), TrustBoundaries: "reviewed"}
			if tc.name == contractKind {
				d.Contracts = []string{cliContract}
				d.ContractChanges = map[string]string{cliContract: compatibleChange}
			} else {
				d.Policies = []string{"model"}
			}
			data, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, designFile, string(data))
			if err := checkChangedDesign(root, "HEAD", c); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreCodeDesignStep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash design step runs in the Linux architecture CI job")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input, rule string
		bad               bool
	}{
		{"valid", ".architecture/design.json", "", false},
		{"forbidden", "tools/architecture/testdata/invalid-design.json", "ARCH002", true},
		{"breaking contract", "tools/architecture/testdata/invalid-contract.json", "ARCH003", true},
		{"prohibited transfer", "tools/architecture/testdata/invalid-trust.json", "ARCH004", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", "scripts/check-design.sh", tc.input) //nolint:gosec // Test inputs are literal repository fixture paths.
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v output=%s", err, output)
			}
			if tc.bad && !strings.Contains(string(output), tc.rule) {
				t.Fatalf("design step did not evaluate the import rule: %s", output)
			}
		})
	}
}

func TestDesignOnlyChangeEvaluated(t *testing.T) {
	root := gitFixture(t)
	writeFile(t, root, designFile, `{"version":1,"issue":76,"summary":"proposal","trust_boundaries":"local","trust_changes":{"checks":{"destinations":["new-service"]}}}`)
	if err := checkChangedDesign(root, "HEAD", testModel(t)); err == nil {
		t.Fatal("incompatible design-only change was not checked")
	}
}

func TestCLIContractSourcesRequireCompatibilityReview(t *testing.T) {
	checkout, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var m model
	if err := loadJSON(checkout, modelFile, &m); err != nil {
		t.Fatal(err)
	}
	c, err := compile(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, contract string }{
		{"cmd/gremlins.go", "cli_persistent"}, {"cmd/internal/flags/flags.go", "cli_binding"},
	} {
		t.Run(tc.contract, func(t *testing.T) {
			root := gitFixture(t)
			writeFile(t, root, tc.file, "package cmd")
			d := design{Version: 1, Issue: 76, Summary: changeSummary, TrustBoundaries: localBoundary, TrustChanges: c.TrustBoundaries, Changes: []change{{Package: packageOf(tc.file), Imports: []string{}}}}
			data, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, designFile, string(data))
			if err := checkChangedDesign(root, "HEAD", c); err == nil {
				t.Fatal("CLI source changed without compatibility review")
			}
			d.Contracts = []string{tc.contract}
			d.ContractChanges = map[string]string{tc.contract: compatibleChange}
			data, err = json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, designFile, string(data))
			if err := checkChangedDesign(root, "HEAD", c); err != nil {
				t.Fatal(err)
			}
		})
	}
}
