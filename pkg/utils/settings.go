package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Settings struct {
	ClaudeCookie string `json:"claudeCookie"`
	CursorCookie string `json:"cursorCookie"`
}

func settingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "tokalytics", "settings.json")
}

func LoadSettings() Settings {
	data, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{}
	}
	var s Settings
	json.Unmarshal(data, &s)
	return s
}

func SaveSettings(s Settings) error {
	path := settingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
