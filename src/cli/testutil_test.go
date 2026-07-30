// Shared test helpers for package cli tests.
package cli

import (
	"io"
	"os"
	"testing"
)

// captureStdout redirects os.Stdout to a pipe for the duration of fn and
// returns everything written to it. Used to assert on the many
// fmt.Println-based help/status/completion outputs in this package.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("pipe close error: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("pipe read error: %v", err)
	}
	return string(out)
}

// withStdin redirects os.Stdin to a pipe pre-loaded with input for the
// duration of fn. Used to drive the bufio.NewReader(os.Stdin)/fmt.Scanln
// confirmation prompts in this package without a real terminal.
func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	go func() {
		_, _ = w.WriteString(input)
		_ = w.Close()
	}()

	fn()
}
