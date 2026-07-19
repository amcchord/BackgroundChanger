package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/amcchord/BackgroundChanger/internal/engine"
	"github.com/amcchord/BackgroundChanger/internal/setup"
	"github.com/amcchord/BackgroundChanger/internal/ui"
	"github.com/amcchord/BackgroundChanger/internal/winservice"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return ui.Main()
	}
	quiet := contains(args, "--quiet")
	switch strings.ToLower(args[0]) {
	case "service":
		return winservice.Run()
	case "--install", "install":
		preset, err := optionValue(args, "--preset")
		if err != nil {
			return err
		}
		if !setup.IsAdministrator() && !quiet {
			elevatedArgs := []string{"--install"}
			if preset != "" {
				elevatedArgs = append(elevatedArgs, "--preset", preset)
			}
			return setup.RelaunchElevated(elevatedArgs...)
		}
		operation := func(progress setup.ProgressFunc) error { return setup.InstallWithPreset(progress, preset) }
		if quiet {
			return operation(nil)
		}
		return ui.RunOperation("Installing BackgroundChanger", operation)
	case "--uninstall", "uninstall":
		if !setup.IsAdministrator() && !quiet {
			return setup.RelaunchElevated("--uninstall")
		}
		operation := func(progress setup.ProgressFunc) error { return setup.Uninstall(progress, false) }
		if quiet {
			return operation(nil)
		}
		return ui.RunOperation("Uninstalling BackgroundChanger", operation)
	case "--render", "render":
		path := "BackgroundChanger-preview.jpg"
		if len(args) > 1 {
			path = args[1]
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		_, err = engine.New(log.New(os.Stderr, "", log.LstdFlags)).RenderPreview(abs)
		if err == nil {
			fmt.Println(abs)
		}
		return err
	case "--refresh", "refresh":
		_, err := engine.New(log.New(os.Stderr, "", log.LstdFlags)).Refresh("manual")
		return err
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func optionValue(values []string, target string) (string, error) {
	for i, value := range values {
		if !strings.EqualFold(value, target) {
			continue
		}
		if i+1 >= len(values) || strings.HasPrefix(values[i+1], "--") {
			return "", fmt.Errorf("%s requires a value", target)
		}
		return values[i+1], nil
	}
	return "", nil
}
