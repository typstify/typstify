package typst

import (
	"context"
	"log"
	"regexp"
	"runtime"
	"strings"

	"looz.ws/typstify/utils"
)

var (
	cmdBuilder utils.CmdBuilder
)

// use init function to setup PATH.
func SetupCmdBuilder(externalExe string) {
	exists, isDir := utils.CheckFileExists(externalExe)
	if exists && !isDir {
		cmdBuilder.Path = externalExe
	} else {
		exeName := "typst"
		if runtime.GOOS == "windows" {
			exeName = "typst.exe"
		}
		cmdBuilder.Path = exeName
	}

	cmdBuilder.DefaultArgs = []string{"--color=never"}
	cmdBuilder.Check()
}

func InitCmd(template string, dir string, opts *InitCmdOptions) error {
	args := []string{"init"}
	args = append(args, opts.Build()...)
	args = append(args, template, dir)

	cmd := cmdBuilder.Build(context.Background(), args...)

	//log.Println("command: ", cmd.String())

	out, err := cmd.Output()
	if len(out) > 0 {
		log.Println("typst init output: ")
		log.Println(string(out))
	}

	return err
}

func QueryCmd() []string {
	return nil
}

func FontsCmd() []string {
	cmd := cmdBuilder.Build(context.Background(), "fonts")
	out, _ := cmd.Output()
	return strings.Split(string(out), "\n")
}

func VersionCmd() string {
	cmd := cmdBuilder.Build(context.Background(), "--version")
	out, _ := cmd.Output()

	pat := regexp.MustCompile(`^typst\s+(\S+)`)
	match := pat.FindSubmatch(out)
	if match == nil {
		return strings.TrimSpace(string(out))
	}

	return string(match[1])
}

var (
	version string
)

func CurrentVersion() string {
	if version == "" {
		version = VersionCmd()
	}

	return version
}
