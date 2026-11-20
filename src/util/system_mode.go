package util

// systemModeAtStart is decided once, at process start and before any privilege drop. System
// versus user directories follow it for the whole process lifetime (AI.md PART 8 "Directory
// mode is locked at process start").
var systemModeAtStart = detectSystemMode()

// IsSystemMode reports whether the process started as root, as an elevated Administrator or
// as a Windows service, and therefore uses the system directories.
func IsSystemMode() bool {
	return systemModeAtStart
}
