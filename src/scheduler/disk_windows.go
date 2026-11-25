//go:build windows

package scheduler

import (
	"fmt"
	"syscall"
	"unsafe"
)

// getDiskUsagePercent returns disk usage percentage for a path
// AI.md: SelfHealthCheck should check disk space
func getDiskUsagePercent(path string) (int, error) {
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

	if totalBytes == 0 {
		return 0, nil
	}

	usedBytes := totalBytes - freeBytesAvailable
	usedPercent := int(float64(usedBytes) / float64(totalBytes) * 100)
	return usedPercent, nil
}

// DiskVolumeID returns the volume root containing path.
func DiskVolumeID(path string) (string, error) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetVolumePathNameW")
	input, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 260)
	ret, _, callErr := proc.Call(uintptr(unsafe.Pointer(input)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if ret == 0 {
		return "", callErr
	}
	return fmt.Sprintf("%s", syscall.UTF16ToString(buffer)), nil
}
