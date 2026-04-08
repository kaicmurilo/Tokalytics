//go:build darwin

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const launchAgentLabel = "io.github.kaicmurilo.tokalytics"

func platformSupported() bool { return true }

func launchAgentsPlistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func platformSet(enable bool, exe string) error {
	plistPath, err := launchAgentsPlistPath()
	if err != nil {
		return err
	}
	uid := strconv.Itoa(os.Getuid())
	domain := "gui/" + uid

	_ = exec.Command("launchctl", "bootout", domain, launchAgentLabel).Run()

	if !enable {
		_ = os.Remove(plistPath)
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return err
	}

	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, escapeXML(launchAgentLabel), escapeXML(exe))

	if err := os.WriteFile(plistPath, []byte(xml), 0644); err != nil {
		return err
	}

	out, err := exec.Command("launchctl", "bootstrap", domain, plistPath).CombinedOutput()
	if err != nil {
		_ = os.Remove(plistPath)
		return fmt.Errorf("launchctl: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
