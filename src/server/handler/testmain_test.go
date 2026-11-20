package handler

import (
	"os"
	"testing"
)

// TestMain clears the directory variables an image or shell may export, so every test
// resolves its directories from the values it sets itself and never touches a real path.
func TestMain(m *testing.M) {
	os.Unsetenv("BACKUP_DIR")
	os.Unsetenv("DATABASE_DIR")
	os.Exit(m.Run())
}
