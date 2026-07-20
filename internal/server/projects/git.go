package projects

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	remoteOperationTimeout = 10 * time.Minute
	localOperationTimeout  = time.Minute
)

type gitEnvironment interface {
	GitEnvironment(background bool) ([]string, error)
}

type commandRunner interface {
	Run(ctx context.Context, env []string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = mergeEnvironment(os.Environ(), env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("run git: %w", err)
	}
	return output, nil
}

func mergeEnvironment(base, overrides []string) []string {
	overrideKeys := make(map[string]struct{}, len(overrides))
	for _, entry := range overrides {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			overrideKeys[key] = struct{}{}
		}
	}
	merged := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if ok {
			if _, overridden := overrideKeys[key]; overridden {
				continue
			}
		}
		merged = append(merged, entry)
	}
	return append(merged, overrides...)
}

func runWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	runner commandRunner,
	env []string,
	args ...string,
) ([]byte, error) {
	operationCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := runner.Run(operationCtx, env, args...)
	if err != nil {
		if errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
			return output, context.DeadlineExceeded
		}
		return output, err
	}
	return output, nil
}
