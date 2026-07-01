package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bamorial/tasker/internal/tasker"
)

func TestRootCommandRunsTUIWithWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	if err := tasker.InitializeWorkspace(root); err != nil {
		t.Fatalf("InitializeWorkspace: %v", err)
	}

	oldRunner := runTUI
	called := false
	var gotRoot string
	runTUI = func(root string) error {
		called = true
		gotRoot = root
		return nil
	}
	t.Cleanup(func() {
		runTUI = oldRunner
	})

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir root: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	rootCmd.SetArgs([]string{})
	rootCmd.SetIn(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetIn(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected TUI runner to be called")
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("EvalSymlinks want root: %v", err)
	}
	gotResolved, err := filepath.EvalSymlinks(gotRoot)
	if err != nil {
		t.Fatalf("EvalSymlinks got root: %v", err)
	}
	if gotResolved != wantRoot {
		t.Fatalf("expected runner root %q, got %q", root, gotRoot)
	}
}
