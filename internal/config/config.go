// Package config defines the administrator-editable YAML service configuration.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	MinRefreshMinutes = 1
	MaxRefreshMinutes = 1440

	PresetIdentity   = "identity"
	PresetBalanced   = "balanced"
	PresetOperations = "operations"
	PresetCustom     = "custom"
)

type Visibility struct {
	OS                 bool `json:"os" yaml:"os"`
	Build              bool `json:"build" yaml:"build"`
	CPU                bool `json:"cpu" yaml:"cpu"`
	GPU                bool `json:"gpu" yaml:"gpu"`
	Memory             bool `json:"memory" yaml:"memory"`
	Disk               bool `json:"disk" yaml:"disk"`
	IP                 bool `json:"ip" yaml:"ip"`
	Serial             bool `json:"serial" yaml:"serial"`
	Uptime             bool `json:"uptime" yaml:"uptime"`
	Services           bool `json:"services" yaml:"services"`
	Restart            bool `json:"restart" yaml:"restart"`
	CriticalServices   bool `json:"critical_services" yaml:"critical_services"`
	FailedAutoServices bool `json:"failed_auto_services" yaml:"failed_auto_services"`
}

type Config struct {
	Preset         string     `json:"preset" yaml:"preset"`
	RefreshMinutes int        `json:"refresh_minutes" yaml:"refresh_minutes"`
	BaseImage      string     `json:"base_image,omitempty" yaml:"base_image"`
	Width          int        `json:"width,omitempty" yaml:"width"`
	Height         int        `json:"height,omitempty" yaml:"height"`
	Show           Visibility `json:"show" yaml:"show"`
}

func Default() Config {
	cfg, _ := ForPreset(PresetBalanced)
	return cfg
}

func ForPreset(name string) (Config, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	cfg := Config{Preset: name, RefreshMinutes: 5}
	switch name {
	case PresetIdentity:
		cfg.Show = Visibility{OS: true, Build: true, IP: true, Serial: true}
	case PresetBalanced:
		cfg.Show = Visibility{
			OS: true, Build: true, CPU: true, GPU: true, Memory: true, Disk: true,
			IP: true, Serial: true, Uptime: true, Services: true, Restart: true,
			CriticalServices: true,
		}
	case PresetOperations:
		cfg.Show = Visibility{
			OS: true, Build: true, CPU: true, Memory: true, Disk: true,
			IP: true, Uptime: true, Services: true, Restart: true,
			CriticalServices: true, FailedAutoServices: true,
		}
	default:
		return Config{}, fmt.Errorf("unknown preset %q", name)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	switch c.Preset {
	case PresetIdentity, PresetBalanced, PresetOperations, PresetCustom:
	default:
		return fmt.Errorf("preset must be identity, balanced, operations, or custom")
	}
	if c.RefreshMinutes < MinRefreshMinutes || c.RefreshMinutes > MaxRefreshMinutes {
		return fmt.Errorf("refresh_minutes must be between %d and %d", MinRefreshMinutes, MaxRefreshMinutes)
	}
	if (c.Width == 0) != (c.Height == 0) {
		return errors.New("width and height must either both be zero or both be set")
	}
	if c.Width != 0 && (c.Width < 800 || c.Width > 7680 || c.Height < 600 || c.Height > 4320) {
		return errors.New("custom dimensions must be between 800x600 and 7680x4320")
	}
	if c.BaseImage != "" && !filepath.IsAbs(c.BaseImage) {
		return errors.New("base_image must be an absolute local path")
	}
	return nil
}

// ValidateAssets verifies external files referenced by a supplied deployment
// configuration before setup changes any service, policy, or installation
// state. Runtime Load intentionally remains tolerant of a temporarily missing
// image so an existing installation can keep its last successful background.
func ValidateAssets(c Config) error {
	if c.BaseImage == "" {
		return nil
	}
	file, err := os.Open(c.BaseImage)
	if err != nil {
		return fmt.Errorf("open base_image: %w", err)
	}
	defer file.Close()
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		return fmt.Errorf("decode base_image: %w", err)
	}
	if format != "jpeg" && format != "png" {
		return fmt.Errorf("base_image must be a JPEG or PNG, got %s", format)
	}
	return nil
}

type diskConfig struct {
	Preset         string      `json:"preset" yaml:"preset"`
	RefreshMinutes *int        `json:"refresh_minutes" yaml:"refresh_minutes"`
	BaseImage      string      `json:"base_image,omitempty" yaml:"base_image,omitempty"`
	Width          int         `json:"width,omitempty" yaml:"width,omitempty"`
	Height         int         `json:"height,omitempty" yaml:"height,omitempty"`
	Show           *Visibility `json:"show" yaml:"show"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return decode(b, false)
}

func decode(b []byte, legacyJSON bool) (Config, error) {
	var raw diskConfig
	var err error
	if legacyJSON {
		err = json.Unmarshal(b, &raw)
	} else {
		decoder := yaml.NewDecoder(bytes.NewReader(b))
		decoder.KnownFields(true)
		err = decoder.Decode(&raw)
	}
	if err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	preset := strings.ToLower(strings.TrimSpace(raw.Preset))
	if preset == "" {
		preset = PresetBalanced
	}
	cfg, presetErr := ForPreset(preset)
	if presetErr != nil {
		if preset != PresetCustom {
			return Config{}, presetErr
		}
		cfg = Config{Preset: PresetCustom, RefreshMinutes: 5}
	}
	if raw.RefreshMinutes != nil {
		cfg.RefreshMinutes = *raw.RefreshMinutes
	}
	cfg.BaseImage, cfg.Width, cfg.Height = raw.BaseImage, raw.Width, raw.Height
	// Named presets are authoritative, so changing only the preset value is a
	// useful power-user shortcut. Individual field overrides intentionally take
	// effect only in custom mode (as documented in every generated file).
	if raw.Show != nil && preset == PresetCustom {
		cfg.Show = *raw.Show
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadOrCreate(path string) (Config, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, nil
	}
	if !os.IsNotExist(err) {
		return Config{}, err
	}
	// Early v3 release candidates used config.json. Import it once into YAML
	// format so upgrades preserve refresh, image, and dimension settings.
	legacyPath := filepath.Join(filepath.Dir(path), "config.json")
	if b, readErr := os.ReadFile(legacyPath); readErr == nil {
		cfg, err = decode(b, true)
		if err != nil {
			return Config{}, fmt.Errorf("migrate config.json: %w", err)
		}
		if err := Save(path, cfg); err != nil {
			return Config{}, err
		}
		return cfg, nil
	}
	cfg = Default()
	if err := Save(path, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := []byte("# Wallpaper Identity (W:ID) power-user configuration.\n# Hostname and the Generated at timestamp are always shown.\n# Set preset to custom after changing individual show values.\n")
	b = append(header, b...)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
