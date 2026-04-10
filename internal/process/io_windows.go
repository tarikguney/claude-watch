//go:build windows

package process

import (
	"fmt"
	"syscall"
	"unsafe"
)

var procGetProcessIoCounters = kernel32.NewProc("GetProcessIoCounters")

// ioCounters matches the Windows IO_COUNTERS structure layout.
type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// GetIOReadBytes returns the cumulative number of bytes read by a process.
func GetIOReadBytes(pid int) (uint64, error) {
	handle, err := syscall.OpenProcess(processQueryInformation, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer syscall.CloseHandle(handle)

	var counters ioCounters
	r, _, err := procGetProcessIoCounters.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&counters)),
	)
	if r == 0 {
		return 0, fmt.Errorf("GetProcessIoCounters(%d): %w", pid, err)
	}
	return counters.ReadTransferCount, nil
}
