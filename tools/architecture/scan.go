package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func excludedDir(name string) bool {
	return slices.Contains([]string{".git", "vendor", "testdata", "dist"}, name)
}

func scan(root string, c *compiledModel) ([]finding, error) {
	var findings []finding
	err := filepath.WalkDir(root, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("incomplete scan; fix: restore read access to the checkout")
		}
		current, err := scanEntry(root, p, entry, c)
		findings = append(findings, current...)

		return err
	})
	if err != nil {
		return nil, err
	}
	findings = append(findings, canonicalFindings(root, c)...)

	slices.SortFunc(findings, compareFindings)

	return findings, nil
}

func scanEntry(root, p string, entry fs.DirEntry, c *compiledModel) ([]finding, error) {
	if entry.IsDir() && p != root && excludedDir(entry.Name()) {
		return nil, filepath.SkipDir
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	if entry.Type()&os.ModeSymlink != 0 {
		return nil, checkLink(root, p, rel)
	}
	if entry.IsDir() {
		return nil, nil
	}
	if entry.Name() == "go.mod" && rel != "go.mod" {
		return nil, fmt.Errorf("nested module %q; fix: classify the nested module explicitly before scanning", rel)
	}
	if !strings.HasSuffix(rel, ".go") {
		return nil, nil
	}

	return scanFile(p, rel, c)
}

func scanFile(absolute, relative string, c *compiledModel) ([]finding, error) {
	pkg := packageOf(relative)
	parsed, err := parser.ParseFile(token.NewFileSet(), absolute, nil, parser.ImportsOnly|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("cannot parse imports in %q; fix: restore valid Go import syntax", relative)
	}
	if _, ok := c.Packages[pkg]; !ok {
		return []finding{{Rule: c.byKind["ownership"].ID, File: relative, Subject: pkg}}, nil
	}
	var findings []finding
	seen := map[string]bool{}
	for _, imp := range parsed.Imports {
		target, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid import in %q; fix: use a valid Go import string", relative)
		}
		if seen[target] {
			return nil, fmt.Errorf("duplicate import in %q; fix: remove the repeated import", relative)
		}
		seen[target] = true
		if target == c.Module || strings.HasPrefix(target, c.Module+"/") {
			dep := strings.TrimPrefix(strings.TrimPrefix(target, c.Module), "/")
			if dep == "" {
				dep = "."
			}
			findings = append(findings, c.dependency(relative, pkg, dep, strings.HasSuffix(relative, "_test.go"))...)
		}
	}

	return findings, nil
}

// Documentation links may point within the checkout; linked Go input is never scanned.
func checkLink(root, absolute, relative string) error {
	target, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return fmt.Errorf("broken link %q; fix: restore its checkout target", relative)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("cannot resolve checkout; fix: restore the checkout path")
	}
	rel, err := filepath.Rel(canonicalRoot, target)
	outside := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
	if err != nil || outside || strings.HasSuffix(relative, ".go") {
		return fmt.Errorf("unsafe link %q; fix: use regular Go files and checkout-local documentation links", relative)
	}

	return nil
}

func canonicalFindings(root string, c *compiledModel) []finding {
	var findings []finding
	for name, file := range c.canonicalPaths() {
		p, err := localPath(root, file)
		if err != nil {
			findings = append(findings, finding{Rule: c.byKind["contracts"].ID, File: file, Subject: name})

			continue
		}
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			findings = append(findings, finding{Rule: c.byKind["contracts"].ID, File: file, Subject: name})
		}
	}

	return findings
}
