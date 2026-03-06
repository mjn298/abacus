package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version  int                      `yaml:"version"`
	Project  ProjectConfig            `yaml:"project"`
	Scanners map[string]ScannerConfig `yaml:"scanners"`
}

type ProjectConfig struct {
	Name string `yaml:"name"`
	Root string `yaml:"root"`
}

type ScannerConfig struct {
	Command string                 `yaml:"command"`
	Options map[string]interface{} `yaml:"options"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Version != 1 {
		return nil, fmt.Errorf("unsupported config version: %d (expected 1)", cfg.Version)
	}

	if cfg.Project.Name == "" {
		return nil, fmt.Errorf("project.name is required")
	}

	return &cfg, nil
}

func DefaultPath() string {
	return ".abacus/config.yaml"
}
