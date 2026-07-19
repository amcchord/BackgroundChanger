// Package engine coordinates gathering, rendering, policy application, and status persistence.
package engine

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/amcchord/WallpaperIdentity/v4/internal/buildinfo"
	"github.com/amcchord/WallpaperIdentity/v4/internal/config"
	"github.com/amcchord/WallpaperIdentity/v4/internal/loginscreen"
	"github.com/amcchord/WallpaperIdentity/v4/internal/overlay"
	"github.com/amcchord/WallpaperIdentity/v4/internal/paths"
	"github.com/amcchord/WallpaperIdentity/v4/internal/sysinfo"
)

type Status struct {
	Version          string                  `json:"version"`
	Reason           string                  `json:"reason"`
	Success          bool                    `json:"success"`
	Error            string                  `json:"error,omitempty"`
	ImagePath        string                  `json:"image_path,omitempty"`
	EditionSupported bool                    `json:"edition_supported"`
	Snapshot         sysinfo.Snapshot        `json:"snapshot"`
	Apply            loginscreen.ApplyResult `json:"apply"`
	CompletedAt      time.Time               `json:"completed_at"`
}

type Engine struct{ Logger *log.Logger }

func New(logger *log.Logger) *Engine { return &Engine{Logger: logger} }

func (e *Engine) Refresh(reason string) (Status, error) {
	started := time.Now()
	e.logf("refresh started: %s", reason)
	cfg, err := config.LoadOrCreate(paths.ConfigFile())
	if err != nil {
		return e.fail(reason, Status{}, fmt.Errorf("load config: %w", err))
	}
	snapshot := sysinfo.Gather()
	status := Status{
		Version: buildinfo.Version, Reason: reason, Snapshot: snapshot,
		EditionSupported: sysinfo.SupportsMachineLockScreenPolicy(snapshot.Edition),
	}
	stamp := snapshot.RefreshedAt.UTC().Format("20060102T150405.000000000")
	imagePath := filepath.Join(paths.ImageDir(), "machine-status-"+stamp+".jpg")
	if err := overlay.RenderToFile(imagePath, snapshot, cfg); err != nil {
		return e.fail(reason, status, fmt.Errorf("render image: %w", err))
	}
	status.ImagePath = imagePath
	status.Apply = loginscreen.Apply(imagePath)
	if !status.Apply.GroupPolicyApplied && !status.Apply.MDMBridgeApplied {
		return e.fail(reason, status, fmt.Errorf("Windows rejected both lock-screen policy methods"))
	}
	status.Success = true
	status.CompletedAt = time.Now()
	if err := writeStatus(status); err != nil {
		e.logf("warning: write status: %v", err)
	}
	cleanupImages(imagePath, 4)
	e.logf("refresh complete in %s: %s", time.Since(started).Round(time.Millisecond), imagePath)
	if !status.EditionSupported {
		e.logf("warning: edition %q does not guarantee machine lock-screen policy support", snapshot.Edition)
	}
	for _, warning := range status.Apply.Warnings {
		e.logf("warning: %s", warning)
	}
	return status, nil
}

func (e *Engine) RenderPreview(path string) (Status, error) {
	cfg, err := config.LoadOrCreate(paths.ConfigFile())
	if err != nil {
		return Status{}, err
	}
	snapshot := sysinfo.Gather()
	status := Status{
		Version: buildinfo.Version, Reason: "preview", Snapshot: snapshot,
		EditionSupported: sysinfo.SupportsMachineLockScreenPolicy(snapshot.Edition), ImagePath: path,
		CompletedAt: time.Now(), Success: true,
	}
	if err := overlay.RenderToFile(path, snapshot, cfg); err != nil {
		return status, err
	}
	return status, nil
}

func (e *Engine) fail(reason string, status Status, err error) (Status, error) {
	status.Version = buildinfo.Version
	status.Reason = reason
	status.Success = false
	status.Error = err.Error()
	status.CompletedAt = time.Now()
	_ = writeStatus(status)
	e.logf("refresh failed: %v", err)
	return status, err
}

func (e *Engine) logf(format string, values ...any) {
	if e.Logger != nil {
		e.Logger.Printf(format, values...)
	}
}

func writeStatus(status Status) error {
	if err := os.MkdirAll(paths.DataDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := paths.StatusFile() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, paths.StatusFile())
}

func cleanupImages(current string, keep int) {
	entries, err := os.ReadDir(paths.ImageDir())
	if err != nil {
		return
	}
	type candidate struct {
		path string
		mod  time.Time
	}
	var files []candidate
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jpg" {
			continue
		}
		info, err := entry.Info()
		if err == nil {
			files = append(files, candidate{path: filepath.Join(paths.ImageDir(), entry.Name()), mod: info.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	retained := 0
	for _, file := range files {
		if samePath(file.path, current) || retained < keep-1 {
			retained++
			continue
		}
		_ = os.Remove(file.path)
	}
}

func samePath(a, b string) bool {
	aAbs, _ := filepath.Abs(a)
	bAbs, _ := filepath.Abs(b)
	return filepath.Clean(aAbs) == filepath.Clean(bAbs)
}
