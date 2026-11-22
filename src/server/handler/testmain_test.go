package handler

import (
	"fmt"
	"os"
	"testing"
)

// TestMain clears the directory variables an image or shell may export, so every test
// resolves its directories from the values it sets itself and never touches a real path.
func TestMain(m *testing.M) {
	for _, key := range []string{"BACKUP_DIR", "DATABASE_DIR"} {
		if err := os.Unsetenv(key); err != nil {
			fmt.Fprintf(os.Stderr, "failed to clear %s: %v\n", key, err)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}
