# ROC Modbus Expert: Technical Guide & Style

## 🛠 Comandos de Desarrollo

```bash
make build           # build-frontend + go build → modbus_client
make build-frontend  # cd frontend && npm ci && npm run build (→ dist/)
make build-go        # go build ./cmd/server/ -o modbus_client
make dev-frontend    # cd frontend && npm run dev (Vite :5173, proxy → :8443)
make dev-backend     # go run ./cmd/server/
make clean
go mod tidy
```

## 🏗 Arquitectura v4.0

### Estructura de paquetes Go
```
cmd/server/main.go          ← entrada: TLS, DB, rutas, embed
internal/
  api/handlers/             ← ws, config, query, roc, sync, raw
  api/router.go
  modbus/                   ← client, protocol, decoders
  logger/                   ← broadcaster, ring buffer, tipos
  config/                   ← YAML load, tipos
  certgen/certs.go          ← auto-genera cert TLS en certs/
  db/                       ← SQLite (modernc.org/sqlite, sin CGO)
web.go                      ← go:embed dist + static (package web)
```

### Frontend Vue 3
```
frontend/
  src/
    stores/    ← connection, logs, modbus, roc, sync (Pinia)
    services/  ← api.js (axios /api), websocket.js (UUID sesión)
    views/     ← QueryView, RocView, SyncView, RawView
    components/layout/, modbus/, roc/, sync/
  tailwind.config.js   ← colores EPM: g, lime, forest
  postcss.config.js    ← tailwindcss + autoprefixer
```

**Sin dependencias CDN** — Tailwind se compila en `dist/assets/index-*.css` vía PostCSS/Vite.
Las fuentes IBM Plex Sans y JetBrains Mono están en `static/fonts/` (autoembebidas).

### Sincronización Paralela
- **POST /api/stations/full-sync** — goroutine por estación, worker pool interno de 2 trabajadores, 840 registros concurrentes.
- **POST /api/stations/partial-sync** — reintenta sólo los `pointers` fallidos.

### HTTPS
- `:8443` HTTPS (cert auto-generado en `certs/` al primer arranque, ECDSA P256, 10 años).
- `:8083` HTTP → redirección a HTTPS.

### SQLite
- `modernc.org/sqlite` (pure Go, sin CGO), archivo `modbus.db`.
- Tablas: `sync_sessions`, `sync_records`, `query_history`.

## ✒️ Guía de Estilo

- **Go**: `go fmt`, camelCase variables, PascalCase exportados.
- **Vue**: Composition API (`<script setup>`), Pinia stores, sin lógica en templates.
- **CSS**: clases utilitarias Tailwind + clases semánticas en `style.css` (`.btn`, `.fi`, `.fs`, `.card`, `.tab`, `.byte`, `.terminal`, `.log-*`).
- **Colores EPM**: verde cítrico `#7AD400` (lime), verde bosque `#007934` (forest), grises `g-*`.
- **Logs**: `BroadcastLog` (todos los clientes) / `SessionBroadcast(sid, msg)` (sesión privada).
- **Seguridad**: sanitizar floats `NaN`/`Inf` → `0` antes de JSON.

## 📂 Archivos Críticos
| Archivo | Propósito |
|---------|-----------|
| `cmd/server/main.go` | Punto de entrada |
| `internal/api/handlers/sync.go` | FullSync + PartialSync + worker pool |
| `internal/modbus/client.go` | `LogFunc` var (rompe ciclo de imports) |
| `internal/logger/types.go` | Interfaz `Client` para WS |
| `web.go` | `go:embed dist static` (package web, raíz del módulo) |
| `frontend/src/stores/sync.js` | Estado sync + handleProgress |
| `frontend/src/services/websocket.js` | UUID sesión + enrutamiento WS |
