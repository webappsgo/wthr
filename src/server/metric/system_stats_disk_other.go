//go:build !unix

// Filesystem usage has no statfs equivalent reachable without CGO or the
// Windows API bindings, so the disk metric family is absent on these platforms.
package metric

// readDiskUsage reports that filesystem statistics are unavailable on this platform.
func readDiskUsage(path string) (diskStat, error) {
	return diskStat{}, errStatsUnsupported
}
