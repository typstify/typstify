package utils

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// lookupExecutable looks up the executable path from the executable dir
// and the process root dir, and existing PATH.
func lookupExecutable(exeName string) string {
	binDir := ""

	currentExePath, err := os.Executable()
	if err == nil {
		binDir = filepath.Dir(currentExePath)
	}

	exePath := filepath.Join(binDir, exeName)
	exists, isDir := CheckFileExists(exePath)

	// Fallback to the process root dir.
	if binDir == "" || !exists || isDir {
		binDir, _ = filepath.Abs(".") // all 3 main OSes are supported.
	}

	exePath = filepath.Join(binDir, exeName)
	exists, isDir = CheckFileExists(exePath)

	if exists && !isDir {
		// update permission to ensure it can be picked up by os.LookPath.
		os.Chmod(exePath, 0755)

		pathEnv := os.Getenv("PATH")
		if runtime.GOOS == "windows" {
			os.Setenv("PATH", binDir+";"+pathEnv)
		} else {
			// linux or macos or any other OS have the same format.
			os.Setenv("PATH", binDir+":"+pathEnv)
		}
	}

	absPath, err := exec.LookPath(exeName)
	if err != nil {
		log.Printf("No %s found after searching PATH: %s", exeName, os.Getenv("PATH"))
		return ""
	}

	return absPath
}

type CmdBuilder struct {
	DefaultArgs []string
	Path        string
}

func (b *CmdBuilder) Build(ctx context.Context, args ...string) *exec.Cmd {
	path := b.Path
	if filepath.Base(path) == path {
		path = lookupExecutable(path)
	} else {
		exists, isDir := CheckFileExists(path)
		if !exists || isDir {
			return nil
		}
	}

	args = append(b.DefaultArgs, args...)
	return BuildCmd(ctx, path, args...)
}
