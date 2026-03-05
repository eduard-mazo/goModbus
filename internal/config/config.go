package config

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	ConfigPath = "config.yaml"
	configMu   sync.RWMutex
)

func LoadConfig(f string) (*Config, error) {
	configMu.RLock()
	defer configMu.RUnlock()
	d, err := os.ReadFile(f)
	if err != nil {
		return &Config{}, nil
	}
	var c Config
	if err := yaml.Unmarshal(d, &c); err != nil {
		return &Config{}, err
	}
	return &c, nil
}
