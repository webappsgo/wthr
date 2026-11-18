//go:build windows

package privilege

// LookupAccount is never used on Windows: the service runs as the Virtual Service
// Account NT SERVICE\{internal_name}, which Windows creates and manages itself.
func LookupAccount(name string) (*Account, error) {
	return nil, errUnsupportedPlatform
}

// EnsureAccount is never used on Windows (AI.md PART 24 "Windows Service Account").
func EnsureAccount(name, home string) (*Account, error) {
	return nil, errUnsupportedPlatform
}
