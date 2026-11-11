//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceClientBinary(t *testing.T) {
	tempBase := filepath.Join(os.TempDir(), projectOrg)
	if err := os.MkdirAll(tempBase, 0755); err != nil {
		t.Fatal(err)
	}
	tempDir, err := os.MkdirTemp(tempBase, projectName+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })
	currentPath := filepath.Join(tempDir, "wthr-cli")
	newPath := filepath.Join(tempDir, "cli.update.tmp")
	if err := os.WriteFile(currentPath, []byte("old"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := replaceClientBinary(currentPath, newPath); err != nil {
		t.Fatalf("replaceClientBinary() error = %v", err)
	}
	contents, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "new" {
		t.Fatalf("binary contents = %q, want new", contents)
	}
	info, err := os.Stat(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("binary mode = %o, want 700", info.Mode().Perm())
	}
}
