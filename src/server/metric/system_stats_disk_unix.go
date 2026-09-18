//go:build unix

// Filesystem usage for the data directory on Unix-like systems via statfs.
package metric

import "syscall"

// readDiskUsage returns total and used bytes for the filesystem holding path.
// Used is computed against the space available to an unprivileged process, so
// the reserved root blocks count as used the way df reports them.
func readDiskUsage(path string) (diskStat, error) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return diskStat{}, err
	}

	blockSize := uint64(fs.Bsize)
	total := uint64(fs.Blocks) * blockSize
	if total == 0 {
		return diskStat{}, errStatsUnsupported
	}

	available := uint64(fs.Bavail) * blockSize
	free := uint64(fs.Bfree) * blockSize

	used := total - free
	usable := used + available

	stat := diskStat{total: total, used: used}
	if usable > 0 {
		stat.usedPercent = float64(used) / float64(usable) * 100
	}

	return stat, nil
}
