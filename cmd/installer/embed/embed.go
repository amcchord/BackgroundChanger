// Package embed contains the embedded bgStatusService.exe binary.
// The binary must be copied to this directory before building the installer.
package embed

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// ServiceExe contains the embedded bgStatusService.exe binary.
// This file must exist in this directory at build time.
//
//go:embed bgStatusService.exe
var ServiceExe []byte

// Version is the version of the embedded service executable.
// This should be updated when the embedded binary is updated.
var Version = "v1.0.0"

// ExtractServiceExe extracts the embedded service executable to a temporary file
// and returns the path to the extracted file.
func ExtractServiceExe() (string, error) {
	if len(ServiceExe) == 0 {
		return "", fmt.Errorf("embedded service executable is empty - build may be corrupted")
	}

	// Create temp file for the executable
	tempDir := os.TempDir()
	destPath := filepath.Join(tempDir, "bgStatusService.exe")

	// Write the embedded binary to the temp file
	err := os.WriteFile(destPath, ServiceExe, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to extract service executable: %w", err)
	}

	return destPath, nil
}

