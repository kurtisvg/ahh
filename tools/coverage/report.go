package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"golang.org/x/tools/cover"
)

const modulePath = "github.com/kurtisvg/ahh"

type totals struct{ covered, statements int64 }

func (t totals) percent() float64 { return 100 * float64(t.covered) / float64(t.statements) }

func isApplicationFile(file string) bool {
	pkg := path.Dir(file)
	switch pkg {
	case modulePath + "/internal/store/testutils", modulePath + "/tools/coverage":
		return false
	}
	return strings.HasPrefix(file, modulePath+"/")
}

func mergeProfiles(files []string) ([]*cover.Profile, error) {
	if len(files) == 0 {
		return nil, errors.New("no coverage profiles supplied")
	}
	var input bytes.Buffer
	input.WriteString("mode: atomic\n")
	for _, file := range files {
		profiles, err := cover.ParseProfiles(file)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", file, err)
		}
		statements := 0
		for _, p := range profiles {
			if p.Mode != "atomic" {
				return nil, fmt.Errorf("%s: expected atomic coverage", file)
			}
			for _, b := range p.Blocks {
				if b.NumStmt < 0 || b.Count < 0 {
					return nil, fmt.Errorf("%s: negative coverage counts", file)
				}
				if !isApplicationFile(p.FileName) {
					continue
				}
				statements += b.NumStmt
				writeProfileBlock(&input, p.FileName, b)
			}
		}
		if statements == 0 {
			return nil, fmt.Errorf("%s: no application statements", file)
		}
	}
	// The Go parser combines identical blocks and rejects inconsistent statement counts.
	return cover.ParseProfilesFromReader(&input)
}

func writeReport(files []string, output string, minimum int, report io.Writer) error {
	if minimum < 0 || minimum > 100 {
		return errors.New("minimum must be between 0 and 100")
	}
	profiles, err := mergeProfiles(files)
	if err != nil {
		return err
	}
	packages := map[string]totals{}
	var total totals
	var merged bytes.Buffer
	merged.WriteString("mode: atomic\n")
	for _, p := range profiles {
		pkg := path.Dir(p.FileName)
		sum := packages[pkg]
		for _, b := range p.Blocks {
			sum.statements += int64(b.NumStmt)
			total.statements += int64(b.NumStmt)
			if b.Count > 0 {
				sum.covered += int64(b.NumStmt)
				total.covered += int64(b.NumStmt)
			}
			writeProfileBlock(&merged, p.FileName, b)
		}
		packages[pkg] = sum
	}
	if total.statements == 0 {
		return errors.New("no application statements")
	}
	if err := os.WriteFile(output, merged.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write merged profile: %w", err)
	}
	if _, err := report.Write(formatSummary(total, packages, minimum)); err != nil {
		return fmt.Errorf("write coverage summary: %w", err)
	}
	if total.covered*100 < int64(minimum)*total.statements {
		return fmt.Errorf("coverage below %d%% minimum", minimum)
	}
	return nil
}

func writeProfileBlock(buffer *bytes.Buffer, file string, block cover.ProfileBlock) {
	fmt.Fprintf(
		buffer,
		"%s:%d.%d,%d.%d %d %d\n",
		file,
		block.StartLine,
		block.StartCol,
		block.EndLine,
		block.EndCol,
		block.NumStmt,
		block.Count,
	)
}

func formatSummary(total totals, packages map[string]totals, minimum int) []byte {
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	var summary bytes.Buffer
	fmt.Fprintf(
		&summary,
		"Go coverage: **%.2f%%** (%d/%d statements); minimum: **%d%%**\n\n",
		total.percent(),
		total.covered,
		total.statements,
		minimum,
	)
	summary.WriteString("| Package | Coverage |\n| --- | ---: |\n")
	for _, name := range names {
		sum := packages[name]
		if sum.statements > 0 {
			fmt.Fprintf(
				&summary,
				"| %s | %.2f%% |\n",
				name,
				sum.percent(),
			)
		}
	}
	return summary.Bytes()
}
