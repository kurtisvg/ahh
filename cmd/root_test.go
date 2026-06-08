package cmd

import "testing"

func TestParseFlags(t *testing.T) {
	t.Parallel()

	opts, err := parseFlags([]string{"--host", "localhost", "--port", "18080", "--version"})
	if err != nil {
		t.Fatal(err)
	}

	if opts.host != "localhost" {
		t.Fatalf("host = %q, want %q", opts.host, "localhost")
	}
	if opts.port != "18080" {
		t.Fatalf("port = %q, want %q", opts.port, "18080")
	}
	if !opts.version {
		t.Fatal("version = false, want true")
	}
}
