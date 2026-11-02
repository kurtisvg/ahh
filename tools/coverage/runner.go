package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type runner struct {
	dir         string
	minimum     int
	stdout      io.Writer
	summaryPath string
	goRun       func(context.Context, ...string) error
}

func collect(ctx context.Context, minimum int, stdout, stderr io.Writer) error {
	moduleFile, err := exec.CommandContext(
		ctx,
		"go",
		"env",
		"GOMOD",
	).Output()
	if err != nil {
		return fmt.Errorf("locate module: %w", err)
	}
	file := strings.TrimSpace(string(moduleFile))
	if file == "" || file == os.DevNull {
		return errors.New("run coverage from inside the ahh module")
	}
	root := filepath.Dir(file)
	output := filepath.Join(root, "coverage")
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	// Never reuse a previous run's counters, including after failed tests.
	dir, err := os.MkdirTemp(output, "run.")
	if err != nil {
		return err
	}
	defer fmt.Fprintln(stdout, "Coverage reports:", dir)
	if err := os.Mkdir(filepath.Join(dir, "subprocess"), 0o755); err != nil {
		return err
	}
	r := runner{
		dir:         dir,
		minimum:     minimum,
		stdout:      stdout,
		summaryPath: os.Getenv("GITHUB_STEP_SUMMARY"),
		goRun: func(ctx context.Context, args ...string) error {
			cmd := exec.CommandContext(ctx, "go", args...)
			cmd.Dir = root
			cmd.Env = append(cmd.Environ(),
				"AHH_TEST_BINARY="+filepath.Join(dir, "ahh"),
				"AHH_COVERAGE_DIR="+filepath.Join(dir, "subprocess"))
			cmd.Stdout, cmd.Stderr = stdout, stderr
			cmd.WaitDelay = 10 * time.Second
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("go %s: %w", strings.Join(args, " "), err)
			}
			return nil
		},
	}
	return r.run(ctx)
}

func (r runner) run(ctx context.Context) error {
	commands := [][]string{
		{"build", "-cover", "-covermode=atomic", "-coverpkg=./...", "-o", filepath.Join(r.dir, "ahh"), "."},
		{"test", "-count=1", "-race", "-shuffle=on", "-timeout", "10m", "-covermode=atomic",
			"-coverpkg=./...", "-coverprofile=" + filepath.Join(r.dir, "tests.out"), "./..."},
		{"tool", "covdata", "textfmt",
			"-i=" + filepath.Join(r.dir, "subprocess"), "-o=" + filepath.Join(r.dir, "subprocess.out")},
	}
	for _, args := range commands {
		if err := r.goRun(ctx, args...); err != nil {
			return err
		}
	}
	var summary bytes.Buffer
	profile := filepath.Join(r.dir, "coverage.out")
	gateErr := writeReport(
		[]string{filepath.Join(r.dir, "tests.out"), filepath.Join(r.dir, "subprocess.out")},
		profile,
		r.minimum,
		&summary,
	)
	// A failed threshold still produces all reports before returning failure.
	reportErr := os.WriteFile(filepath.Join(r.dir, "summary.md"), summary.Bytes(), 0o600)
	_, printErr := r.stdout.Write(summary.Bytes())
	var appendErr error
	if r.summaryPath != "" {
		appendErr = appendSummary(r.summaryPath, summary.Bytes())
	}
	var htmlErr error
	if _, err := os.Stat(profile); err == nil {
		htmlErr = r.goRun(
			ctx,
			"tool",
			"cover",
			"-html="+profile,
			"-o="+filepath.Join(r.dir, "coverage.html"),
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		htmlErr = err
	}
	return errors.Join(
		gateErr,
		reportErr,
		printErr,
		appendErr,
		htmlErr,
	)
}

func appendSummary(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open job summary: %w", err)
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}
