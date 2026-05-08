//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// attachWindowsConsole attaches to the parent terminal's console so CLI commands
// can print output when compiled with -H windowsgui (no console by default).
// When started without a parent console (registry autostart, Explorer) the call
// is a no-op — correct for systray background mode.
// Only called when len(os.Args) > 1 (CLI mode), never for systray mode.
func attachWindowsConsole() {
	const attachParentProcess = ^uint32(0) // ATTACH_PARENT_PROCESS = -1

	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	attachConsole := kernel32.NewProc("AttachConsole")
	setStdHandle := kernel32.NewProc("SetStdHandle")

	ret, _, _ := attachConsole.Call(uintptr(attachParentProcess))
	if ret == 0 {
		return // no parent console
	}

	out, err := windows.CreateFile(
		windows.StringToUTF16Ptr("CONOUT$"),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, 0, 0,
	)
	if err != nil {
		return
	}

	const (
		stdOutputHandle = uintptr(0xFFFFFFF5) // STD_OUTPUT_HANDLE
		stdErrorHandle  = uintptr(0xFFFFFFF4) // STD_ERROR_HANDLE
	)
	setStdHandle.Call(stdOutputHandle, uintptr(out))
	setStdHandle.Call(stdErrorHandle, uintptr(out))

	f := os.NewFile(uintptr(out), "stdout")
	os.Stdout = f
	os.Stderr = f
}
