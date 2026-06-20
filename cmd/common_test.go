package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/cobra"
)

func invokeCommand(t *testing.T, args []string) (*cobra.Command, string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	executed, err := cmd.ExecuteC()
	return executed, buf.String(), err
}

func invokeCommandWithoutRun(t *testing.T, args []string) (*cobra.Command, string, error) {
	t.Helper()

	buf := new(bytes.Buffer)
	cmd := newRootCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	if target, _, err := cmd.Find(args); err == nil {
		target.Run = nil
		target.RunE = func(cmd *cobra.Command, args []string) error {
			if target.Args != nil {
				return target.Args(cmd, args)
			}
			return nil
		}
	}
	cmd.SetArgs(args)
	executed, err := cmd.ExecuteC()
	return executed, buf.String(), err
}

func flagString(t *testing.T, cmd *cobra.Command, name string) string {
	t.Helper()

	value, err := cmd.Flags().GetString(name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
