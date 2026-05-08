//go:build !windows

package hud

// ShowHUD is a no-op on non-Windows platforms.
func ShowHUD() {}
