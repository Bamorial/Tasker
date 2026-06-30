package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommandPrintsGreetingBeforeHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs([]string{})

	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	if err := Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	output := stdout.String()
	if !strings.HasPrefix(output, "Hello!\n") {
		t.Fatalf("expected output to start with greeting, got %q", output)
	}

	if !strings.Contains(output, "Usage:\n  tasker [flags]\n  tasker [command]") {
		t.Fatalf("expected root help output, got %q", output)
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr output, got %q", stderr.String())
	}
}
