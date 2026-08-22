//go:build windows

package backup

import (
	"syscall"
	"unsafe"
)

// VolumeTotalBytes returns the total capacity of the filesystem containing
// path, used to resolve a percent-based retention.max_total_size cap and the
// disk_threshold check in the disk-space guard per AI.md PART 22.
func VolumeTotalBytes(path string) (int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes int64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		return 0, syscall.GetLastError()
	}
	return totalBytes, nil
}

// VolumeFreeBytes returns the free space available to an unprivileged
// process on the filesystem containing path, used by the scheduled backup
// task's disk-space guard (AI.md PART 22: "backup.skipped_disk_full").
func VolumeFreeBytes(path string) (int64, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceEx := kernel32.NewProc("GetDiskFreeSpaceExW")

	var freeBytesAvailable, totalBytes, totalFreeBytes int64

	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}

	ret, _, _ := getDiskFreeSpaceEx.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)

	if ret == 0 {
		return 0, syscall.GetLastError()
	}
	return freeBytesAvailable, nil
}
