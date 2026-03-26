package utils

import (
	"errors"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/sys/windows"
)

func CheckFileExists(path string) (exists bool, isDir bool) {
	info, err := os.Stat(path)
	if err == nil {
		return true, info.IsDir()
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, false
	}

	return false, false
}

func OpenInExternalApp(path string) error {
	switch runtime.GOOS {
	case "darwin", "ios":
		return runCmd("open", path)
	case "windows":
		// explorer command returns non-zero even if it is successful.
		// So we migrate to the shell API here.
		return openInWindowsExplorer(path)
		// return runCmd("explorer", path)
	default:
		// linux, unix flavors.
		return runCmd("xdg-open", path)
	}
}

func openInWindowsExplorer(path string) error {
	verbPtr, _ := windows.UTF16PtrFromString("open")
	pathPtr, _ := windows.UTF16PtrFromString(path)

	return windows.ShellExecute(0, verbPtr, pathPtr, nil, nil, windows.SW_SHOWNORMAL)
}

func runCmd(cmdName string, arg ...string) error {
	cmd := exec.Command(cmdName, arg...)
	return cmd.Run()
}
