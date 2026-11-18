//go:build windows

package privilege

// ChownTree is never used on Windows: directory access is granted to the Virtual
// Service Account through ACLs by the service installer.
func ChownTree(root string, account *Account) error {
	return errUnsupportedPlatform
}
