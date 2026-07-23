package projects

import (
	"context"
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

type gitEnv interface {
	Env() ([]string, error)
}

type commandRunner interface {
	Run(ctx context.Context, env []string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, env []string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	env = mergeEnvironment(env, []string{
		"GIT_TERMINAL_PROMPT=0",
		"GCM_INTERACTIVE=Never",
	})
	cmd.Env = mergeEnvironment(os.Environ(), env)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return output, contextErr
		}
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
