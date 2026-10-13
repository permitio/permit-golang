package main

import (
	"os"
	"runtime"
	"testing"
)

func TestFootprintKiB(t *testing.T) {
	before, err := footprintKiB(os.Getpid())
	if err != nil || before <= 0 {
		t.Fatalf("got %d, %v", before, err)
	}
	block := make([]byte, 64<<20)
	for i := 0; i < len(block); i += pageBytes {
		block[i] = 1
	}
	after, err := footprintKiB(os.Getpid())
	runtime.KeepAlive(block)
	if err != nil || after-before < 60<<10 {
		t.Errorf("64 MiB more in use, footprint %d KiB then %d KiB (%v)", before, after, err)
	}
	if _, err := footprintKiB(0x7ffffff0); err == nil {
		t.Error("no error for a process that does not exist")
	}
}
