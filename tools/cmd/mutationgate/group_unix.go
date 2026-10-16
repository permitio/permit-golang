//go:build unix

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// startInGroup makes cmd start its process in a process group of its own, which the
// processes it starts join, so that killGroup reaches every one of them.
func startInGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killGroup sends SIGKILL to every process in the group of the process cmd started,
// whose ID is that process's PID: the kernel does not give the PID to another process
// while the group has a member. kill reports a group that is gone as ESRCH, and one
// that holds no process it may signal as EPERM, such as, on macOS, a group whose
// members have all exited but are not yet reaped. Neither is an error, as there is
// nothing more that exec could kill. killGroup also kills the process itself, so
// that it does not run on when it has left the group.
func killGroup(cmd *exec.Cmd) error {
	groupErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if errors.Is(groupErr, syscall.ESRCH) || errors.Is(groupErr, syscall.EPERM) {
		groupErr = nil
	}
	if groupErr != nil {
		groupErr = fmt.Errorf("its process group: %w", groupErr)
	}
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		err = nil
	}
	return errors.Join(groupErr, err)
}

// notifyStop relays the signals that would end exec to the channel it returns: an
// interrupt, a hangup, SIGTERM, and SIGQUIT, which go test sends to exec when the
// test runs past go test's own deadline. In a group of its own, the test process gets
// none of them from the terminal or from a kill of exec's group, so exec stops the
// group itself. It leaves out a signal that exec started with ignored, as nohup
// leaves a hangup: the test process inherits that signal as ignored.
//
// notifyStop also takes SIGPIPE, so that a write to exec's stdout or stderr after
// the go command that reads them has exited fails with EPIPE, which watchedWriter
// handles, instead of ending exec and leaving the test process's group running.
func notifyStop() <-chan os.Signal {
	signals := make(chan os.Signal, 1)
	stop := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT}
	for _, sig := range stop {
		if !signal.Ignored(sig) {
			signal.Notify(signals, sig)
		}
	}
	signal.Notify(make(chan os.Signal, 1), syscall.SIGPIPE)
	return signals
}

// isClosedPipe reports whether err is EPIPE, the error of a write to a pipe that no
// process reads.
func isClosedPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
