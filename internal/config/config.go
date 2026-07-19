// Package config defines the small, administrator-editable service configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	MinRefreshMinutes = 1
	MaxRefreshMinutes = 1440
)

type Config struct {
	RefreshMinutes int    `json:"refresh_minutes"`
	BaseImage      string `json:"base_image,omitempty"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
}

func Default() Config {
	return Config{RefreshMinutes: 5}
}

func (c Config) Validate() error {
	if c.RefreshMinutes < MinRefreshMinutes || c.RefreshMinutes > MaxRefreshMinutes {
		return fmt.Errorf("refresh_minutes must be between %d and %d", MinRefreshMinutes, MaxRefreshMinutes)
	}
	if (c.Width == 0) != (c.Height == 0) {
		return errors.New("width and height must either both be zero or both be set")
	}
	if c.Width != 0 && (c.Width < 800 || c.Width > 7680 || c.Height < 600 || c.Height > 4320) {
		return errors.New("custom dimensions must be between 800x600 and 7680x4320")
	}
	return nil
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
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
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
