# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build    # Compile to ./modbus_client
make run      # Build and run the server (http://localhost:8081)
make dev      # Run with race detector
make check    # go fmt + go vet
make deps     # go mod tidy
make clean    # Remove compiled binary

go test ./...                  # Run all tests
go vet ./...                   # Static analysis
go build -o modbus_client .    # Direct build
```

Port is **8081**. Binary name is `modbus_client` (no `.exe`).

## Architecture

Single `package main` across five Go source files + one self-contained HTML frontend.

| File | Responsibility |
|---|---|
| `main.go` | Gin server setup, route wiring, WS broadcaster goroutine |
| `modbus.go` | `ModbusClient`, `Execute()`, PDU/MBAP builders, decoders, FC01–FC16 |
| `handlers.go` | HTTP handlers: `queryHandler`, `rocHandler`, `rocHistory24Handler`, `wsHandler`, `getConfigHandler` |
| `logger.go` | `broadcastLog()`, in-memory ring buffer (500 entries), file logger (`logs/modbus_debug.log`), `clientsMu` |
| `config.go` | `Config`, `StationConfig` types, `loadConfig()` |
| `index.html` | Alpine.js 3.14 + Tailwind — **offline, served from `static/`** |

**API endpoints:**
- `GET  /api/config` — station presets from `config.yaml`
- `POST /api/query` — generic Modbus query (FC01–FC16); returns `req_hex`, `res_hex`, `registers[]`, `floats[]`, `coils[]`
- `POST /api/roc` — ROC two-step flow (pointer → history); modes: `ptr | hist | full`
- `POST /api/roc/history24` — 24-hour circular buffer fetch
- `GET  /ws` — WebSocket log stream

**Static assets (offline, no CDN):**
- `static/alpine.min.js` — Alpine.js 3.14.1
- `static/tailwind.min.js` — Tailwind CDN play script
- `static/fonts.css` + `static/fonts/*.woff2` — IBM Plex Sans + JetBrains Mono

### Modbus Client (`modbus.go`)

`Execute(fc, addr, qty, writeData)` is the single generic method. It calls `buildPDU()` and `buildMBAP()`, measures RTT, and returns `(data, sentBytes, elapsed, error)`. Read FCs (01–04) return payload bytes; write FCs (05, 06, 0F, 10) echo back the confirmation.

Four endianness modes for float decoding: `abcd` (BE), `dcba` (LE), `cdab` (word-swap, ROC default), `badc` (byte-swap).

### WebSocket Broadcast Pattern

A single goroutine started in `main()` reads from `logChan` and fans out to all connected `wsClientConn` structs (each has its own buffered `chan LogMessage`). This avoids the single-consumer channel problem.

### ROC Read Flow (`rocHandler`)

1. **`ptr` or `full`**: FC03 at `ptr_addr` with `ptr_qty` registers, decode as float32 (CDAB) or int16 → `ptr_value`
2. **`hist` or `full`**: FC03 at `db_addr + uint16(ptr_value)` with Qty=1 (ROC protocol constraint)

### ROC 24h Circular Buffer (`rocHistory24Handler`)

```
startPtr = (currentPtr − currentHour + bufSize) % bufSize
ptr[h]   = (startPtr + h) % bufSize
```

Buffer size: 840 slots (0–839). If `db_qty ≥ 3`, the current hour is read from `register[2]` of the device record; otherwise system clock is used.

### Frontend (`index.html`)

**Stack:** Alpine.js 3.14 + Tailwind CSS (both served from `static/` — no CDN, works offline).
**EPM brand colors:** verde cítrico `#7AD400` (Pantone 375), verde bosque `#007934` (Pantone 355), gray `#666366` (Cool gray 11).
**Fonts:** IBM Plex Sans (UI) + JetBrains Mono (data/hex) — self-hosted in `static/fonts/`.

Two tabs: **Consulta Modbus** (SimplyModbus-style: FC selector, address, qty, ADU preview, byte-box display, register table with hex/dec/signed/binary columns, float32 grid, coils grid) and **ROC Expert** (pointer + DB config, mode selector, result cards, 24h history bar chart + table).

Sidebar: connection params (IP, port, slave ID, endianness) + station presets from `config.yaml`.
