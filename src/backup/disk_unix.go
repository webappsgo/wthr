//go:build !windows

package backup

import "syscall"

// volumeTotalBytes returns the total capacity of the filesystem containing
// path, used to resolve a percent-based retention.max_total_size cap per
// AI.md PART 22.
func volumeTotalBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Blocks) * int64(stat.Bsize), nil
}
