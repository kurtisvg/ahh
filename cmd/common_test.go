package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type commandTestCase struct {
	name            string
	args            []string
	setup           func(*testing.T)
	timeout         time.Duration
	wantErr         error
	wantOutContains []string
	wantErrOut      []string
	wantErrContains string
}

func runCommandTest(t *testing.T, tt commandTestCase) {
	t.Helper()

	t.Run(tt.name, func(t *testing.T) {
		if tt.setup != nil {
			tt.setup(t)
		}

		ctx := t.Context()
		if tt.timeout != 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, tt.timeout)
			defer cancel()
		}

		stdout, stderr, err := executeRootCommand(ctx, tt.args...)

		if tt.wantErrContains == "" && tt.wantErr == nil && err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
			t.Fatalf("Execute() error = %v, want %v", err, tt.wantErr)
		}
		if tt.wantErrContains != "" {
			if err == nil {
				t.Fatal("Execute() error = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErrContains)
			}
		}

		for _, want := range tt.wantOutContains {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout = %q, want containing %q", stdout, want)
			}
		}
		for _, want := range tt.wantErrOut {
			if !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want containing %q", stderr, want)
			}
		}
		if len(tt.wantErrOut) == 0 && stderr != "" {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})
}
