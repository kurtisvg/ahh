package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/kurtisvg/ahh/internal/wrapper"
	"github.com/spf13/cobra"
)

type wrapOpts struct {
	address string
	port    int
}

type wrapStartup struct {
	Address string `json:"address,omitempty"`
	Port    int    `json:"port,omitempty"`
	Error   string `json:"error,omitempty"`
}

func newWrapCommand() *cobra.Command {
	var opts wrapOpts
	cmd := &cobra.Command{
		Use:       "wrap <harness-name>",
		Short:     "Run the local harness wrapper",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"codex"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWrapCommand(cmd, opts, args[0])
		},
	}

	cmd.Flags().StringVar(&opts.address, "address", "127.0.0.1", "Address to bind")
	cmd.Flags().IntVar(&opts.port, "port", 8081, "Port to bind (0 chooses an available port)")
	return cmd
}

func runWrapCommand(cmd *cobra.Command, opts wrapOpts, name string) error {
	ctx := cmd.Context()
	server, err := wrapper.Start(
		ctx,
		opts.address,
		opts.port,
		name,
	)
	if err != nil {
		// Start returns only after failed setup has released its resources. This
		// explicit report lets the orchestrator distinguish a safe retry from a
		// wrapper that disappeared while it may still own a harness.
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(wrapStartup{Error: err.Error()})
		return fmt.Errorf("start wrapper server: %w", err)
	}
	defer func() {
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
		_ = server.Wait()
	}()

	started := wrapStartup{
		Address: server.Address,
		Port:    server.Port,
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(started); err != nil {
		return fmt.Errorf("report wrapper startup: %w", err)
	}
	address := net.JoinHostPort(server.Address, strconv.Itoa(server.Port))
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "wrapper listening at http://%s\n", address); err != nil {
		return err
	}

	if err := server.Wait(); err != nil {
		return fmt.Errorf("serve wrapper: %w", err)
	}

	// Stdout carries lifecycle messages; terminal output stays on the WebSocket.
	finished := struct {
		Type   string             `json:"type"`
		Result wrapper.ExitResult `json:"result"`
	}{
		Type:   "exit",
		Result: server.Result(),
	}
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(finished); err != nil {
		return fmt.Errorf("report harness exit: %w", err)
	}
	return nil
}
