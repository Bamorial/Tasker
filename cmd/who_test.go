package cmd

import (
	"bytes"
	"testing"
)

func TestWhoCommandPrintsTaskerMe(t *testing.T) {
	cmd := whoCmd
	cmd.SetArgs(nil)
	cmd.SetIn(nil)

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(nil)

	if err := cmd.RunE(cmd, []string{}); err != nil {
		t.Fatalf("whoCmd.RunE: %v", err)
	}

	if got := stdout.String(); got != "tasker me\n" {
		t.Fatalf("expected %q, got %q", "tasker me\n", got)
	}
}
