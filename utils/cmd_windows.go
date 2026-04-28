package utils

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func BuildCmd(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	return cmd
}

// explorer command returns non-zero even if it is successful.
// So we migrate to the shell API here.
func OpenInExternalApp(path string) error {
	verbPtr, _ := windows.UTF16PtrFromString("open")
	pathPtr, _ := windows.UTF16PtrFromString(path)

	return windows.ShellExecute(0, verbPtr, pathPtr, nil, nil, windows.SW_SHOWNORMAL)
}
