//go:build !windows

package scheduler

import (
	"fmt"
	"syscall"
)

// getDiskUsagePercent returns disk usage percentage for a path
// AI.md: SelfHealthCheck should check disk space
func getDiskUsagePercent(path string) (int, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}

	// Calculate used percentage
	totalBytes := int64(stat.Blocks) * int64(stat.Bsize)
	freeBytes := int64(stat.Bavail) * int64(stat.Bsize)

	if totalBytes == 0 {
		return 0, nil
	}

	usedPercent := int(float64(totalBytes-freeBytes) / float64(totalBytes) * 100)
	return usedPercent, nil
}

// DiskVolumeID returns the filesystem device identifier containing path.
func DiskVolumeID(path string) (string, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(path, &stat); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d", stat.Dev), nil
}
