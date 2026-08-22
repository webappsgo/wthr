//go:build !windows

// Tests for disk_unix.go per AI.md PART 22 (Backup & Restore) / PART 29 (Testing)
package backup

import "testing"

// TestVolumeTotalAndFreeBytes verifies both syscall-backed helpers return a
// positive byte count for a real, existing directory, and that free never
// exceeds total on the same filesystem.
func TestVolumeTotalAndFreeBytes(t *testing.T) {
	dir := t.TempDir()

	total, err := VolumeTotalBytes(dir)
	if err != nil {
		t.Fatalf("VolumeTotalBytes() error = %v", err)
	}
	if total <= 0 {
		t.Errorf("VolumeTotalBytes() = %d, want > 0", total)
	}

	free, err := VolumeFreeBytes(dir)
	if err != nil {
		t.Fatalf("VolumeFreeBytes() error = %v", err)
	}
	if free <= 0 {
		t.Errorf("VolumeFreeBytes() = %d, want > 0", free)
	}

	if free > total {
		t.Errorf("VolumeFreeBytes() = %d exceeds VolumeTotalBytes() = %d", free, total)
	}
}

// TestVolumeTotalAndFreeBytesMissingPath verifies both helpers return an
// error (rather than a false zero value that a caller might misread as "no
// space") when the path does not exist.
func TestVolumeTotalAndFreeBytesMissingPath(t *testing.T) {
	missing := "/nonexistent/path/for/wthr/backup/disk/test"

	if _, err := VolumeTotalBytes(missing); err == nil {
		t.Error("VolumeTotalBytes() on a missing path should return an error")
	}
	if _, err := VolumeFreeBytes(missing); err == nil {
		t.Error("VolumeFreeBytes() on a missing path should return an error")
	}
}
