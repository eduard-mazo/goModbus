// Package web holds the embedded frontend assets for the ROC Modbus Expert server.
// The dist/ directory is populated by `make build-frontend` (Vite build).
// The static/ directory contains fonts and shared CSS.
package web

import "embed"

//go:embed dist
var DistFS embed.FS

//go:embed static
var StaticFS embed.FS
