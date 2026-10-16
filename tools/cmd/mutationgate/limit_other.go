//go:build !linux && !darwin

package main

import (
	"fmt"
	"runtime"
)

// limitMemory fails: exec limits memory only on Linux and macOS.
func limitMemory(int64) (func(pid int) (int64, error), error) {
	return nil, fmt.Errorf("no memory limit on %s", runtime.GOOS)
}
