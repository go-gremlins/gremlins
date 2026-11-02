// Command architecture checks repository boundaries and proposed designs.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/OpenRAE/go-rae-gremlins/internal/log"
)

const modelFile = ".architecture/model.json"
const baselineFile = ".architecture/baseline.json"
const designFile = ".architecture/design.json"

func main() {
	log.Init(os.Stdout, os.Stderr)
	base := flag.String("base", "origin/dev", "Git comparison commit or ref (empty means first adoption)")
	proposed := flag.String("design", "", "check a proposed design instead of source")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Errorln("unexpected arguments; fix: use -base or -design")
		os.Exit(1)
	}
	if err := run(*base, *proposed); err != nil {
		log.Errorln(err)
		os.Exit(1)
	}
}

func run(base, proposed string) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get checkout: %w", err)
	}
	var m model
	if err := loadJSON(root, modelFile, &m); err != nil {
		return err
	}
	c, err := compile(m)
	if err != nil {
		return err
	}
	data, err := readLocal(root, modelFile)
	if err != nil {
		return err
	}
	if proposed != "" {
		var d design
		if err := loadJSON(root, proposed, &d); err != nil {
			return err
		}
		if err := checkDesign(c, d); err != nil {
			return err
		}
		ids := make([]string, 0, len(c.byID))
		for id := range c.byID {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		log.Infof("Design #%d passed: model SHA256 %x; rules %q. Review contract and trust assertions against the implementation.\n", d.Issue, sha256.Sum256(data), strings.Join(ids, ", "))

		return nil
	}

	return checkRepository(root, base, c)
}

func checkRepository(root, base string, c *compiledModel) error {
	var candidate baseline
	if err := loadJSON(root, baselineFile, &candidate); err != nil {
		return err
	}
	if err := validateBaseline(candidate, c); err != nil {
		return err
	}
	target, revision, err := targetBaseline(root, base)
	if err != nil {
		return err
	}
	if err := validateBaseline(target, c); err != nil {
		return err
	}
	findings, err := scan(root, c)
	if err != nil {
		return err
	}
	for _, f := range findings {
		log.Infoln(formatFinding(f, c))
	}
	if err := ratchet(findings, candidate.Findings, target.Findings); err != nil {
		return err
	}
	if revision != "" {
		if err := checkChangedDesign(root, revision, c); err != nil {
			return err
		}
	}
	log.Infof("Architecture passed: %d baseline findings; comparison %q.\n", len(findings), revision)

	return nil
}
