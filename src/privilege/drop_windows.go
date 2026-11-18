//go:build windows

package privilege

import "errors"

var errUnsupportedPlatform = errors.New("not supported on Windows: the service runs as a Virtual Service Account")

// IsRoot is always false on Windows: there is no UID 0 and no privilege drop.
func IsRoot() bool {
	return false
}

// DropPrivileges is never used on Windows (AI.md PART 24 "Windows Service Account").
func DropPrivileges(account *Account) error {
	return errUnsupportedPlatform
}
