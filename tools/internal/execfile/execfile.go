// Package execfile writes files that the program, or a process it starts, runs next.
//
// On Linux, running a file fails with "text file busy" (ETXTBSY) while any process has it open
// for writing, and a process that another goroutine starts while the file is open keeps a copy
// of the descriptor until it execs (https://go.dev/issue/22315). syscall.ForkExec, which starts
// every process os/exec runs, holds syscall.ForkLock for writing while it forks, so WriteFile
// holds it for reading from open to close: no process starts while the file is open.
package execfile

import (
	"io/fs"
	"os"
	"syscall"
)

// WriteFile writes data to the file name as os.WriteFile does, while this program starts no
// process. It creates the file with perm (before the umask), or truncates it without changing
// its mode. Every file that a program in this module, or its tests, writes and then runs is
// written with it.
func WriteFile(name string, data []byte, perm fs.FileMode) error {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	return os.WriteFile(name, data, perm)
}
