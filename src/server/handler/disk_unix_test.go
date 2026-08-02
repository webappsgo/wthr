//go:build !windows
// +build !windows

package handler

import "testing"

// TestGetDiskUsage_ValidPath verifies getDiskUsage returns sane, internally
// consistent stats for a path that is guaranteed to exist on any Unix host.
func TestGetDiskUsage_ValidPath(t *testing.T) {
	usage := getDiskUsage("/")

	if usage.Path != "/" {
		t.Errorf("expected Path %q, got %q", "/", usage.Path)
	}
	if usage.TotalBytes <= 0 {
		t.Fatalf("expected TotalBytes > 0 for a real filesystem, got %d", usage.TotalBytes)
	}
	if usage.FreeBytes < 0 {
		t.Errorf("expected FreeBytes >= 0, got %d", usage.FreeBytes)
	}
	if usage.UsedBytes != usage.TotalBytes-usage.FreeBytes {
		t.Errorf("expected UsedBytes = TotalBytes - FreeBytes (%d), got %d",
			usage.TotalBytes-usage.FreeBytes, usage.UsedBytes)
	}
	if usage.UsedPercent < 0 || usage.UsedPercent > 100 {
		t.Errorf("expected UsedPercent in [0,100], got %d", usage.UsedPercent)
	}
}

// TestGetDiskUsage_InvalidPath verifies the statfs-failure fallback path
// returns a zeroed DiskUsage rather than panicking or returning garbage.
func TestGetDiskUsage_InvalidPath(t *testing.T) {
	invalidPath := "/this/path/almost-certainly-does-not-exist-wthr-test"
	usage := getDiskUsage(invalidPath)

	if usage.Path != invalidPath {
		t.Errorf("expected Path %q, got %q", invalidPath, usage.Path)
	}
	if usage.UsedBytes != 0 || usage.FreeBytes != 0 || usage.TotalBytes != 0 || usage.UsedPercent != 0 {
		t.Errorf("expected all-zero DiskUsage for an invalid path, got %+v", usage)
	}
}
