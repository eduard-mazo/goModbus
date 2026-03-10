package config

import "goModbus/internal/modbus"

// Config holds the full application configuration
type Config struct {
	Stations []StationConfig `yaml:"stations" json:"stations"`
}

// MedidorConfig describes a single meter (medidor) within a station.
// Used when one ROC device contains multiple independent flow meters,
// each with its own circular-buffer base address and/or pointer register.
type MedidorConfig struct {
	Label          int    `yaml:"label"             json:"label"`
	Name           string `yaml:"name"              json:"name"`
	PointerAddress uint16 `yaml:"pointer_address"   json:"pointer_address"`
	DBAddress      uint16 `yaml:"base_data_address" json:"base_data_address"`
}

// StationConfig describes a pre-configured ROC device.
// If Medidores is non-empty the per-medidor addresses take precedence over
// the station-level PointerAddress / DBAddress (which serve as defaults).
type StationConfig struct {
	Name               string            `yaml:"name"                 json:"name"`
	IP                 string            `yaml:"ip"                   json:"ip"`
	Port               int               `yaml:"port"                 json:"port"`
	ID                 byte              `yaml:"id"                   json:"id"`
	Endian             modbus.Endianness `yaml:"endian"               json:"endian"`
	PointerAddress     uint16            `yaml:"pointer_address"      json:"pointer_address"`
	DBAddress          uint16            `yaml:"base_data_address"    json:"base_data_address"`
	DataRegistersCount uint16            `yaml:"data_registers_count" json:"data_registers_count"`
	DataType           string            `yaml:"data_type"            json:"data_type"`
	Medidores          []MedidorConfig   `yaml:"medidores"            json:"medidores,omitempty"`
	SignalNames        []string          `yaml:"signal_names"         json:"signal_names,omitempty"`
}
