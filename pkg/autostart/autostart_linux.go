//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func platformSupported() bool { return true }

func autostartDesktopPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "autostart", "tokalytics.desktop"), nil
}

func platformSet(enable bool, exe string) error {
	p, err := autostartDesktopPath()
	if err != nil {
		return err
	}
	if !enable {
		_ = os.Remove(p)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	execLine := exe
	if strings.ContainsAny(exe, " \t\"'\\") {
		execLine = `"` + strings.ReplaceAll(exe, `"`, `\"`) + `"`
	}
	content := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=Tokalytics\nExec=%s\nTerminal=false\nHidden=false\nX-GNOME-Autostart-enabled=true\n", execLine)
	return os.WriteFile(p, []byte(content), 0644)
}
