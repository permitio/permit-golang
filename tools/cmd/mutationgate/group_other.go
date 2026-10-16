//go:build !unix

package main

import (
	"errors"
	"os"
	"os/exec"
)

// startInGroup leaves cmd as it is: there are no process groups here.
func startInGroup(*exec.Cmd) {}

// killGroup kills only the process cmd started: without process groups, the
// processes it started are left running. A process that is done is not an error.
func killGroup(cmd *exec.Cmd) error {
	err := cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}

// notifyStop returns nil: exec handles no signal here.
func notifyStop() <-chan os.Signal {
	return nil
}

// isClosedPipe reports false: exec does not look for a closed pipe here, so Wait
// returns any error in writing its output.
func isClosedPipe(error) bool {
	return false
}
