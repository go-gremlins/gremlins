package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

// gitRead uses fixed commands and never runs hooks, shell code, or generators.
func gitRead(root string, args ...string) ([]byte, error) {
	fixed := append([]string{"-C", root, "--no-pager", "-c", "core.fsmonitor=false"}, args...)
	cmd := exec.Command("git", fixed...) //nolint:gosec // Fixed Git commands; refs are resolved with --end-of-options.
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git history read failed; fix: fetch the pinned comparison commit and retry")
	}

	return data, nil
}

func targetBaseline(root, ref string) (baseline, string, error) {
	empty := baseline{Version: 1, Findings: []finding{}}
	if ref == "" {
		return empty, "", nil
	}
	raw, err := gitRead(root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return baseline{}, "", err
	}
	revision := strings.TrimSpace(string(raw))
	listing, err := gitRead(root, "ls-tree", revision, "--", baselineFile)
	if err != nil {
		return baseline{}, "", err
	}
	if len(listing) == 0 {
		return empty, revision, nil
	}
	// A tree symlink or submodule must never become policy data.
	if !bytes.HasPrefix(listing, []byte("100644 blob ")) && !bytes.HasPrefix(listing, []byte("100755 blob ")) {
		return baseline{}, "", fmt.Errorf("invalid target baseline mode; fix: restore a regular baseline file")
	}
	data, err := gitRead(root, "show", revision+":"+baselineFile)
	if err != nil {
		return baseline{}, "", err
	}
	var b baseline
	if err := decode(data, &b); err != nil {
		return baseline{}, "", err
	}

	return b, revision, nil
}

func changedPaths(root, revision string) ([]string, error) {
	data, err := gitRead(root, "diff", "--no-ext-diff", "--no-textconv", "--name-only", "-z", revision, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := gitRead(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	data = append(data, untracked...)
	var paths []string
	for _, p := range strings.Split(string(data), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}

	return paths, nil
}

func changedPackage(file string) bool {
	for _, part := range strings.Split(file, "/") {
		if excludedDir(part) {
			return false
		}
	}

	return strings.HasSuffix(file, ".go")
}

func checkChangedDesign(root, revision string, c *compiledModel) error {
	paths, err := changedPaths(root, revision)
	if err != nil {
		return err
	}
	var packages []string
	for _, p := range paths {
		if changedPackage(p) {
			pkg := packageOf(p)
			if _, exists := c.Packages[pkg]; exists {
				packages = append(packages, pkg)
			}
		}
	}
	contracts := changedOwners(paths, c.Contracts)
	policies := changedOwners(paths, c.Policies)
	if len(packages) == 0 && len(contracts) == 0 && len(policies) == 0 && !slices.Contains(paths, designFile) {
		return nil
	}
	if !slices.Contains(paths, designFile) {
		return fmt.Errorf(
			"modeled changes need an updated design; fix: update %s and run the pre-code design check",
			designFile,
		)
	}
	var d design
	if err := loadJSON(root, designFile, &d); err != nil {
		return err
	}
	if err := checkDesign(c, d); err != nil {
		return err
	}

	return checkDesignCoverage(d, packages, contracts, policies)
}

func checkDesignCoverage(d design, packages, contracts, policies []string) error {
	for _, name := range contracts {
		if !slices.Contains(d.Contracts, name) {
			return fmt.Errorf("contract %q lacks design coverage; fix: declare its compatibility review in %s", name, designFile)
		}
	}
	for _, name := range policies {
		if !slices.Contains(d.Policies, name) {
			return fmt.Errorf("policy %q lacks design coverage; fix: declare its control review in %s", name, designFile)
		}
	}
	for _, pkg := range packages {
		if !slices.ContainsFunc(d.Changes, func(item change) bool { return item.Package == pkg }) {
			return fmt.Errorf("changed package %q lacks design coverage; fix: declare it in %s before coding", pkg, designFile)
		}
	}

	return nil
}

func changedOwners(paths []string, owners map[string]string) []string {
	var changed []string
	for name, p := range owners {
		if slices.Contains(paths, p) {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)

	return changed
}
