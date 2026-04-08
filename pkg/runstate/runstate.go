package runstate

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const fileName = "runstate.json"

type State struct {
	PID  int `json:"pid"`
	Port int `json:"port"`
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "tokalytics", fileName), nil
}

func Read() (State, error) {
	var s State
	p, err := path()
	if err != nil {
		return s, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return s, nil
}

func Write(pid, port int) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(State{PID: pid, Port: port}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

func Remove() {
	p, err := path()
	if err != nil {
		return
	}
	_ = os.Remove(p)
}
