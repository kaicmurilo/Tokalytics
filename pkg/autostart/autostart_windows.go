//go:build windows

package autostart

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

const runValueName = "Tokalytics"

func platformSupported() bool { return true }

func platformSet(enable bool, exe string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("registry: %w", err)
	}
	defer k.Close()

	if !enable {
		_ = k.DeleteValue(runValueName)
		return nil
	}
	return k.SetStringValue(runValueName, exe)
}
