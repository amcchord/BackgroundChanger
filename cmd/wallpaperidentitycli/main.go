package main

import (
	"os"

	"github.com/amcchord/WallpaperIdentity/v4/internal/application"
)

func main() {
	os.Exit(application.MainCLI(os.Args[1:], os.Stdout, os.Stderr))
}
