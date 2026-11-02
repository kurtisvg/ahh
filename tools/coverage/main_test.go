package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoverageGate(t *testing.T) {
	for _, tt := range []struct {
		name      string
		minimum   int
		wantError bool
	}{
		{"below", 49, false}, {"equal", 50, false}, {"above", 51, true},
		{"invalid", 101, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			// Duplicate hits must not inflate the numerator; an untested package counts.
			data := []string{
				"mode: atomic\n" + module + "/a/a.go:1.1,2.1 2 1\n" + module + "/b/b.go:1.1,2.1 2 0\n",
				"mode: atomic\n" + module + "/a/a.go:1.1,2.1 2 4\n" + module + "/internal/store/testutils/x.go:1.1,2.1 99 1\n",
			}
			files := []string{}
			for i, content := range data {
				file := filepath.Join(dir, string(rune('a'+i))+".out")
				if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				files = append(files, file)
			}
			var report bytes.Buffer
			output := filepath.Join(dir, "merged.out")
			err := run(files, output, tt.minimum, &report)
			if (err != nil) != tt.wantError {
				t.Fatalf("error = %v", err)
			}
			if tt.minimum <= 100 && !strings.Contains(report.String(), "50.00%") {
				t.Fatal(report.String())
			}
		})
	}
}

func TestMergeRejectsInvalidProfiles(t *testing.T) {
	for _, tt := range []struct{ name, content string }{
		{"empty", ""}, {"header only", "mode: atomic\n"},
		{"malformed", "mode: atomic\nbroken\n"},
		{"wrong mode", "mode: set\n" + module + "/a.go:1.1,2.1 1 1\n"},
		{"negative", "mode: atomic\n" + module + "/a.go:1.1,2.1 1 -1\n"},
		{"inconsistent", "mode: atomic\n" + module + "/a.go:1.1,2.1 1 1\n" + module + "/a.go:1.1,2.1 2 1\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "input.out")
			if err := os.WriteFile(file, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := merge([]string{file}); err == nil {
				t.Fatal("accepted invalid profile")
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		if _, err := merge([]string{filepath.Join(t.TempDir(), "missing")}); err == nil {
			t.Fatal("accepted missing profile")
		}
	})
	t.Run("no inputs", func(t *testing.T) {
		if _, err := merge(nil); err == nil {
			t.Fatal("accepted no profiles")
		}
	})
}

func TestApplicationExclusions(t *testing.T) {
	for _, pkg := range []string{"internal/store/testutils", "tools/coverage"} {
		if application(module + "/" + pkg + "/file.go") {
			t.Errorf("included %s", pkg)
		}
	}
	if application("example.org/dep/a.go") || !application(module+"/main.go") {
		t.Fatal("incorrect application selection")
	}
}

func TestWeightedSubprocessCoverage(t *testing.T) {
	dir := t.TempDir()
	unit := filepath.Join(dir, "unit.out")
	child := filepath.Join(dir, "child.out")
	// An uncovered large package outweighs a completely covered tiny package.
	for file, data := range map[string]string{
		unit:  "mode: atomic\n" + module + "/tiny/a.go:1.1,2.1 1 1\n" + module + "/large/a.go:1.1,2.1 9 0\n",
		child: "mode: atomic\n" + module + "/large/a.go:1.1,2.1 9 1\n",
	} {
		if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var report bytes.Buffer
	output := filepath.Join(dir, "merged.out")
	if err := run([]string{unit}, output, 11, &report); err == nil {
		t.Fatal("used unweighted package average")
	}
	report.Reset()
	if err := run([]string{unit, child}, output, 100, &report); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.String(), "10/10 statements") {
		t.Fatal(report.String())
	}
	// A failed gate must still leave a usable report/profile.
	if _, err := merge([]string{output}); err != nil {
		t.Fatal(err)
	}
}

func TestThresholdDoesNotRoundUp(t *testing.T) {
	file := filepath.Join(t.TempDir(), "input.out")
	data := "mode: atomic\n" + module + "/a.go:1.1,2.1 83999 1\n" + module + "/a.go:3.1,4.1 16001 0\n"
	if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	var report bytes.Buffer
	if err := run([]string{file}, filepath.Join(t.TempDir(), "merged.out"), 84, &report); err == nil {
		t.Fatal("rounded up to threshold")
	}
	if !strings.Contains(report.String(), "84.00%") {
		t.Fatal(report.String())
	}
}
