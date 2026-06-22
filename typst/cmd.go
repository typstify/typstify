package typst

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"regexp"
	"runtime"
	"strconv"
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

type FontVariant struct {
	Style   string `json:"style"`
	Weight  int    `json:"weight"`
	Stretch string `json:"stretch"`
}

type FontFamily struct {
	Name     string        `json:"name"`
	Variants []FontVariant `json:"variants,omitempty"`
}

// FontCmd runs `typst fonts` command in the editor environment and return parsed
// font families.
//
// Command sample output:
//
//	Grantha Sangam MN
//	- Style: Normal, Weight: 400, Stretch: FontStretch(1000)
//	- Style: Normal, Weight: 700, Stretch: FontStretch(1000)
//	Gujarati MT
//	- Style: Normal, Weight: 400, Stretch: FontStretch(1000)
//	- Style: Normal, Weight: 700, Stretch: FontStretch(1000)
//	Gujarati Sangam MN
//	- Style: Normal, Weight: 400, Stretch: FontStretch(1000)
//	- Style: Normal, Weight: 700, Stretch: FontStretch(1000)
//
// If variants is not passed, no variant list is returned.
func FontsCmd(ctx context.Context, opts *FontCmdOptions) ([]FontFamily, error) {
	args := []string{"fonts"}
	args = append(args, opts.Build()...)

	cmd := cmdBuilder.Build(ctx, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))

	var variantRE = regexp.MustCompile(
		`^- Style:\s*(.+?),\s*Weight:\s*(\d+),\s*Stretch:\s*(.+)$`,
	)

	fonts := make([]FontFamily, 0)
	var current *FontFamily

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "- ") {
			// start of a new font
			if current != nil {
				fonts = append(fonts, *current)
			}
			current = &FontFamily{Name: line}
			continue
		}

		if current == nil {
			return nil, fmt.Errorf("variant without family: %q", line)
		}

		// parse varient line
		m := variantRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("invalid variant line: %q", line)
		}

		weight, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, fmt.Errorf("invalid variant weight: %q", m[2])
		}

		current.Variants = append(current.Variants, FontVariant{
			Style:   m[1],
			Weight:  weight,
			Stretch: m[3],
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return fonts, nil
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
