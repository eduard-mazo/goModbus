package config

import "goModbus/internal/modbus"

// Config holds the full application configuration
type Config struct {
	Stations []StationConfig `yaml:"stations" json:"stations"`
}

// StationConfig describes a pre-configured ROC device
type StationConfig struct {
	Name               string           `yaml:"name"                 json:"name"`
	IP                 string           `yaml:"ip"                   json:"ip"`
	Port               int              `yaml:"port"                 json:"port"`
	ID                 byte             `yaml:"id"                   json:"id"`
	Endian             modbus.Endianness `yaml:"endian"               json:"endian"`
	PointerAddress     uint16           `yaml:"pointer_address"      json:"pointer_address"`
	DBAddress          uint16           `yaml:"base_data_address"    json:"base_data_address"`
	DataRegistersCount uint16           `yaml:"data_registers_count" json:"data_registers_count"`
	DataType           string           `yaml:"data_type"            json:"data_type"`
}
