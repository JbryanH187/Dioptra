//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

const (
	winDetachedProcess       = 0x00000008
	winCreateNewProcessGroup = 0x00000200
)

// configureDetachedDaemon desacopla el proceso `dioptra serve` de la consola y grupo
// de procesos de la sesión MCP padre, evitando que Windows lo cierre cuando termina la CLI.
func configureDetachedDaemon(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: winDetachedProcess | winCreateNewProcessGroup,
		HideWindow:    true,
	}
}
