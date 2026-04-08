//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func setDetachChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
