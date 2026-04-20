package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IsWSL reports whether we run Linux under WSL (kernel string from Microsoft).
func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	b, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	s := strings.ToLower(string(b))
	return strings.Contains(s, "microsoft") || strings.Contains(s, "wsl")
}

// WindowsHomeOnWSL returns the Windows user profile path as seen from WSL (e.g. /mnt/c/Users/you).
// Set TOKALYTICS_WINDOWS_HOME to override. Empty if not under WSL or path missing.
func WindowsHomeOnWSL() string {
	if !IsWSL() {
		return ""
	}
	if custom := strings.TrimSpace(os.Getenv("TOKALYTICS_WINDOWS_HOME")); custom != "" {
		if st, err := os.Stat(custom); err == nil && st.IsDir() {
			return filepath.Clean(custom)
		}
		return ""
	}
	for _, u := range []string{
		strings.TrimSpace(os.Getenv("USER")),
		strings.TrimSpace(os.Getenv("USERNAME")),
	} {
		if u == "" {
			continue
		}
		p := filepath.Join("/mnt/c/Users", u)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return filepath.Clean(p)
		}
	}
	return ""
}

// DataHomeRoots returns home directories where tools store ~/.claude-style data.
// Under WSL, includes the Windows profile when it exists and differs from the Linux home,
// so Tokalytics (Linux binary) still sees Cursor/Claude installed for Windows.
func DataHomeRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(p string) {
		p = filepath.Clean(p)
		if p == "" {
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	add(home)
	if wh := WindowsHomeOnWSL(); wh != "" && filepath.Clean(wh) != filepath.Clean(home) {
		add(wh)
	}
	return out
}
