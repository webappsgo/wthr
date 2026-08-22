// Shared test helpers for package cli tests.
package cli

import (
	"database/sql"
	"io"
	"os"
	"testing"
)

// applySchema creates dbPath and executes one of the real schema constants
// (database.ServerSchema / database.UsersSchema) against it. Fixtures always
// use the shipped schema rather than a hand-rolled CREATE TABLE so a query
// naming a table or column that no live schema creates fails the test instead
// of passing against a table only the test knows about.
func applySchema(t *testing.T, dbPath, schema string) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("applySchema: sql.Open error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("applySchema: exec error = %v", err)
	}
}

// execFixture runs a single seed statement against an existing fixture
// database, binding every value as a parameter.
func execFixture(t *testing.T, dbPath, query string, args ...interface{}) {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("execFixture: sql.Open error = %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("execFixture: exec error = %v", err)
	}
}

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
