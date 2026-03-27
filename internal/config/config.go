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

func SaveConfig(f string, cfg *Config) error {
	configMu.Lock()
	defer configMu.Unlock()
	d, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(f, d, 0644)
}

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
