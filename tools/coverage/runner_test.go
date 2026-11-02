package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerReportsOnThresholdFailure(t *testing.T) {
	dir := t.TempDir()
	data := "mode: atomic\n" + module + "/a.go:1.1,2.1 1 1\n" + module + "/a.go:3.1,4.1 1 0\n"
	for _, name := range []string{"tests.out", "subprocess.out"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	jobSummary := filepath.Join(dir, "job.md")
	if err := os.WriteFile(jobSummary, []byte("Earlier job output\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	htmlGenerated := false
	r := runner{
		dir: dir, minimum: 84, stdout: &output, summaryPath: jobSummary,
		goRun: func(_ context.Context, args ...string) error {
			if args[0] == "tool" && args[1] == "cover" {
				htmlGenerated = true
			}
			return nil
		},
	}
	err := r.run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "below 84%") {
		t.Fatalf("error = %v", err)
	}
	if !htmlGenerated {
		t.Fatal("failed gate skipped HTML generation")
	}
	if !strings.Contains(output.String(), "50.00%") {
		t.Fatal(output.String())
	}
	for _, name := range []string{"coverage.out", "summary.md"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || len(data) == 0 {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	summary, err := os.ReadFile(jobSummary)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(summary), "Earlier job output\n") || !bytes.Contains(summary, []byte("50.00%")) {
		t.Fatalf("job summary = %s", summary)
	}
}

func TestRunnerStopsOnCommandFailure(t *testing.T) {
	for _, stage := range []string{"build", "test", "covdata"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New("command failed")
			failed := false
			dir := t.TempDir()
			r := runner{dir: dir, minimum: 84, stdout: &bytes.Buffer{},
				goRun: func(_ context.Context, args ...string) error {
					if failed {
						t.Fatal("continued after failed command")
					}
					name := args[0]
					if name == "tool" {
						name = args[1]
					}
					if name == stage {
						failed = true
						return failure
					}
					return nil
				},
			}
			if err := r.run(t.Context()); !errors.Is(err, failure) {
				t.Fatalf("error = %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "coverage.out")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("failed tests produced a passing coverage report")
			}
		})
	}
}
