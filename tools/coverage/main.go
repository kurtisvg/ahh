// Command coverage collects and checks application statement coverage.
package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/tools/cover"
)

const module = "github.com/kurtisvg/ahh"

//go:embed minimum.txt
var minimumText string

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := command(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func command(ctx context.Context, args []string) error {
	minimum, err := strconv.Atoi(strings.TrimSpace(minimumText))
	if err != nil || minimum < 0 || minimum > 100 {
		return errors.New("minimum.txt must contain a whole percentage between 0 and 100")
	}
	if len(args) == 0 {
		return collect(ctx, minimum)
	}
	if args[0] == "-h" || args[0] == "--help" {
		fmt.Println("Usage: go run ./tools/coverage [report [-minimum N] [-out FILE] PROFILE...]")
		fmt.Println("Without arguments, run the full coverage suite and enforce minimum.txt.")
		return nil
	}
	if args[0] != "report" {
		return fmt.Errorf("unknown argument %q; use --help for usage", args[0])
	}
	flags := flag.NewFlagSet("coverage report", flag.ContinueOnError)
	floor := flags.Int("minimum", minimum, "minimum whole percentage (0-100)")
	output := flags.String("out", "coverage.out", "merged profile")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	return run(flags.Args(), *output, *floor, os.Stdout)
}

func application(file string) bool {
	pkg := path.Dir(file)
	switch pkg {
	case module + "/internal/store/testutils", module + "/tools/coverage":
		return false
	}
	return strings.HasPrefix(file, module+"/")
}

func merge(files []string) ([]*cover.Profile, error) {
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
				if !application(p.FileName) {
					continue
				}
				statements += b.NumStmt
				fmt.Fprintf(&input, "%s:%d.%d,%d.%d %d %d\n", p.FileName,
					b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, b.Count)
			}
		}
		if statements == 0 {
			return nil, fmt.Errorf("%s: no application statements", file)
		}
	}
	// The Go parser combines identical blocks and rejects inconsistent statement counts.
	return cover.ParseProfilesFromReader(&input)
}

type totals struct{ covered, statements int64 }

func (t totals) percent() float64 { return 100 * float64(t.covered) / float64(t.statements) }

func run(files []string, output string, minimum int, report io.Writer) error {
	if minimum < 0 || minimum > 100 {
		return errors.New("minimum must be between 0 and 100")
	}
	profiles, err := merge(files)
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
			fmt.Fprintf(&merged, "%s:%d.%d,%d.%d %d %d\n", p.FileName,
				b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, b.Count)
		}
		packages[pkg] = sum
	}
	if total.statements == 0 {
		return errors.New("no application statements")
	}
	if err := os.WriteFile(output, merged.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write merged profile: %w", err)
	}
	names := make([]string, 0, len(packages))
	for name := range packages {
		names = append(names, name)
	}
	sort.Strings(names)
	var summary bytes.Buffer
	fmt.Fprintf(&summary, "Go coverage: **%.2f%%** (%d/%d statements); minimum: **%d%%**\n\n", total.percent(), total.covered, total.statements, minimum)
	summary.WriteString("| Package | Coverage |\n| --- | ---: |\n")
	for _, name := range names {
		sum := packages[name]
		if sum.statements > 0 {
			fmt.Fprintf(&summary, "| %s | %.2f%% |\n", name, sum.percent())
		}
	}
	if _, err := report.Write(summary.Bytes()); err != nil {
		return err
	}
	if total.covered*100 < int64(minimum)*total.statements {
		return fmt.Errorf("coverage below %d%% minimum", minimum)
	}
	return nil
}
