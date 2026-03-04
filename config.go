package main

import (
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds the full application configuration
type Config struct {
	Stations []StationConfig `yaml:"stations" json:"stations"`
}

// StationConfig describes a pre-configured ROC device
type StationConfig struct {
	Name               string     `yaml:"name"                 json:"name"`
	IP                 string     `yaml:"ip"                   json:"ip"`
	Port               int        `yaml:"port"                 json:"port"`
	ID                 byte       `yaml:"id"                   json:"id"`
	Endian             Endianness `yaml:"endian"               json:"endian"`
	PointerAddress     uint16     `yaml:"pointer_address"      json:"pointer_address"`
	DBAddress          uint16     `yaml:"base_data_address"    json:"base_data_address"`
	DataRegistersCount uint16     `yaml:"data_registers_count" json:"data_registers_count"`
	DataType           string     `yaml:"data_type"            json:"data_type"`
}

var (
	configPath = "config.yaml"
	configMu   sync.RWMutex
)

func loadConfig(f string) (*Config, error) {
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
