//go:build windows

package main

import "os/exec"

func setDetachChild(cmd *exec.Cmd) {
	_ = cmd
}
