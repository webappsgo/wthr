//go:build !windows

package privilege

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ChownTree gives the account ownership of root and everything below it (AI.md PART 8
// step 8c). Symbolic links are changed themselves, never followed, so a link inside a
// service directory can never move ownership of a file elsewhere on the system.
func ChownTree(root string, account *Account) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := os.Lchown(path, account.UID, account.GID); err != nil {
			return fmt.Errorf("changing owner of %s: %w", path, err)
		}
		return nil
	})
}
