//go:build !windows

package util

import "github.com/webappsgo/wthr/src/privilege"

// detectSystemMode reports whether the process started as root.
func detectSystemMode() bool {
	return privilege.StartedAsRoot()
}
