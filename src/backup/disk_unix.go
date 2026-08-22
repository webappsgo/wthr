//go:build !windows

package backup

import "syscall"

// VolumeTotalBytes returns the total capacity of the filesystem containing
// path, used to resolve a percent-based retention.max_total_size cap and the
// disk_threshold check in the disk-space guard per AI.md PART 22.
func VolumeTotalBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Blocks) * int64(stat.Bsize), nil
}

// VolumeFreeBytes returns the free space available to an unprivileged
// process on the filesystem containing path, used by the scheduled backup
// task's disk-space guard (AI.md PART 22: "backup.skipped_disk_full").
func VolumeFreeBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
