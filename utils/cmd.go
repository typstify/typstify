//go:build !windows
// +build !windows

package utils

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

func BuildCmd(ctx context.Context, path string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(cmd.Env, os.Environ()...)
	return cmd
}

func OpenInExternalApp(path string) error {
	switch runtime.GOOS {
	case "darwin", "ios":
		return runCmd("open", path)
	default:
		// linux, unix flavors.
		return runCmd("xdg-open", path)
	}
}

func runCmd(cmdName string, arg ...string) error {
	cmd := exec.Command(cmdName, arg...)
	return cmd.Run()
}
