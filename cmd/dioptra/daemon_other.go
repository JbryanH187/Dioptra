//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func configureDetachedDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
