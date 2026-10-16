package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// limitMemory returns the memory reader exec polls. macOS does not enforce
// RLIMIT_AS (ulimit -v), and it compresses idle pages, which then leave a
// process's resident size, so exec stops a test process by its physical
// footprint, which counts compressed pages too: what Activity Monitor shows.
func limitMemory(int64) (func(pid int) (int64, error), error) {
	return footprintKiB, nil
}

// The proc_info system call's arguments for proc_pid_rusage (sys/proc_info.h and
// sys/resource.h), which libproc wraps and the syscall package does not.
const (
	procInfoCallPIDRusage = 9
	rusageFlavorV0        = 0
)

// rusageInfo is struct rusage_info_v0.
type rusageInfo struct {
	uuid              [16]byte
	userTime          uint64
	systemTime        uint64
	pkgIdleWakeups    uint64
	interruptWakeups  uint64
	pageins           uint64
	wiredSize         uint64
	residentSize      uint64
	physFootprint     uint64
	processStartTime  uint64
	processExitedTime uint64
}

// footprintKiB returns a process's physical footprint in KiB.
func footprintKiB(pid int) (int64, error) {
	var info rusageInfo
	//nolint:gosec // G103: proc_info writes the struct; the syscall package does not wrap it.
	_, _, errno := syscall.Syscall6(syscall.SYS_PROC_INFO, procInfoCallPIDRusage, uintptr(pid),
		rusageFlavorV0, 0, uintptr(unsafe.Pointer(&info)), 0)
	if errno != 0 {
		return 0, fmt.Errorf("proc_pid_rusage %d: %w", pid, errno)
	}
	return int64(info.physFootprint >> 10), nil
}
