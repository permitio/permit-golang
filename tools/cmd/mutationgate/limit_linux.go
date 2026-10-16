package main

import (
	"fmt"
	"syscall"
)

// limitMemory caps the address space of this process, and so of the test process
// it starts and that process's children, at mib MiB (RLIMIT_AS, as ulimit -v
// does). An allocation past it fails, and the Go runtime exits with "out of
// memory". The kernel enforces it, so there is nothing to poll.
func limitMemory(mib int64) (func(pid int) (int64, error), error) {
	limit := uint64(mib) << 20 //nolint:gosec // G115: parseExecArgs rejects a mib below 1.
	rlimit := syscall.Rlimit{Cur: limit, Max: limit}
	if err := syscall.Setrlimit(syscall.RLIMIT_AS, &rlimit); err != nil {
		return nil, fmt.Errorf("setrlimit RLIMIT_AS: %w", err)
	}
	return nil, nil //nolint:nilnil // No memory reader: the kernel enforces the limit.
}
