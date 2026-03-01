# ROC Modbus Expert · EPM

Herramienta de ingeniería industrial para diagnóstico, consulta y monitoreo de controladores ROC mediante Modbus TCP.
Desarrollada para **EPM (Empresas Públicas de Medellín)**.

---

## Características

- **Consulta Modbus genérica** — FC01 a FC16 con vista SimplyModbus: previsualización del ADU en vivo, tabla de registros (HEX / DEC / signed / binario), grid de Float32 y cuadrícula de coils
- **ROC Expert** — flujo nativo de dos pasos (lectura de puntero → lectura histórica con Qty=1)
- **Historial 24 horas** — recorrido automático del buffer circular ROC (840 slots) con cálculo del puntero de inicio para cada hora del día
- **4 modos de endianness** — ABCD, DCBA, CDAB (Word-Swap, estándar ROC), BADC
- **WebSocket log** — terminal en tiempo real con filtro por nivel (INFO / DEBUG / ERROR)
- **Presets de estaciones** — preconfiguraciones en `config.yaml`
- **Totalmente offline** — todos los assets del frontend están embebidos en `static/`; sin dependencias de CDN ni internet en tiempo de ejecución

---

## Requisitos

- Go 1.24+
- Navegador moderno (Chrome, Firefox, Edge)

---

## Inicio rápido

```bash
# 1. Compilar
make build

# 2. (Opcional) configurar estaciones en config.yaml

# 3. Ejecutar
./modbus_client

# 4. Abrir en el navegador
http://localhost:8081
```

Ver `make help` para todos los targets disponibles.

---

## Configuración (`config.yaml`)

```yaml
stations:
  - name: "Estación 1"
    ip: "10.155.150.201"
    port: 502
    id: 10
    pointer_address: 10000      # Dirección del registro puntero
    data_registers_count: 2     # Qty para leer el puntero (2 regs = float32)
    pointer_endian: "cdab"      # CDAB = Word-Swap (estándar ROC)
    db_address: 700             # Dirección base de la BD histórica
    db_endian: "cdab"
```

### Opciones de endianness

| Código | Orden | Uso típico |
|--------|-------|-----------|
| `cdab` | Word-Swap | ROC estándar |
| `abcd` | Big-Endian | Modbus estándar |
| `dcba` | Little-Endian | — |
| `badc` | Byte-Swap | — |

---

## API

| Método | Ruta | Descripción |
|--------|------|-------------|
| `GET`  | `/api/config` | Presets de estaciones desde `config.yaml` |
| `POST` | `/api/query`  | Consulta Modbus genérica (FC01–FC16) |
| `POST` | `/api/roc`    | Flujo ROC: puntero + histórico (modos: `ptr`, `hist`, `full`) |
| `POST` | `/api/roc/history24` | Historial 24h usando buffer circular ROC |
| `GET`  | `/ws`         | WebSocket — stream de log en tiempo real |

### POST `/api/query`

```json
{
  "ip": "192.168.1.1",
  "port": 502,
  "slave_id": 1,
  "fc": 3,
  "start_address": 100,
  "quantity": 10,
  "endianness": "abcd",
  "write_data_hex": ""
}
```

Respuesta: `req_hex`, `res_hex`, `registers[]`, `floats[]`, `coils[]`, `elapsed_ms`, `byte_count`

### POST `/api/roc`

```json
{
  "ip": "10.155.150.201",
  "port": 502,
  "slave_id": 10,
  "ptr_addr": 10000,
  "ptr_qty": 2,
  "ptr_endian": "cdab",
  "db_addr": 700,
  "db_endian": "cdab",
  "mode": "full"
}
```

Modos: `ptr` (solo puntero), `hist` (solo histórico con `manual_ptr`), `full` (ambos)

### POST `/api/roc/history24`

```json
{
  "ip": "10.155.150.201",
  "port": 502,
  "slave_id": 10,
  "ptr_addr": 10000,
  "ptr_qty": 2,
  "ptr_endian": "cdab",
  "db_addr": 700,
  "db_qty": 2,
  "db_endian": "cdab",
  "buf_size": 840,
  "current_hour": 14
}
```

`current_hour` es opcional; si se omite el servidor usa la hora del sistema.

---

## Buffer circular ROC — matemática

El buffer histórico cicla entre los slots `0` y `buf_size - 1` (típicamente 840).

```
startPtr = (currentPtr − currentHour + bufSize) % bufSize
ptr[h]   = (startPtr + h) % bufSize
```

**Ejemplo:** `currentPtr = 1`, `currentHour = 2`

```
startPtr = (1 − 2 + 840) % 840 = 839   → representa las 00:00
ptr[0] = 839  → 00:00
ptr[1] =   0  → 01:00
ptr[2] =   1  → 02:00  ← hora actual
```

Si `db_qty ≥ 3`, el servidor lee la hora directamente del registro `[2]` del registro actual del dispositivo; de lo contrario usa el reloj del sistema.

---

## Arquitectura

```
goModbus/
├── main.go        Servidor Gin, rutas, goroutine de broadcast WS
├── modbus.go      ModbusClient — Execute(), buildPDU/MBAP, decoders (FC01–FC16)
├── handlers.go    queryHandler, rocHandler, rocHistory24Handler, wsHandler, getConfigHandler
├── logger.go      broadcastLog(), ring buffer (500 entradas), log a archivo
├── config.go      Config / StationConfig — loadConfig()
├── index.html     Frontend: Alpine.js 3.14 + Tailwind (offline, sin build step)
├── config.yaml    Presets de estaciones
├── static/
│   ├── alpine.min.js     Alpine.js 3.14.1 (local)
│   ├── tailwind.min.js   Tailwind CDN play (local)
│   ├── fonts.css         @font-face declarations (IBM Plex Sans + JetBrains Mono)
│   └── fonts/            Archivos WOFF2 de las fuentes
└── logs/
    └── modbus_debug.log  Log persistente con timestamps de microsegundos
```

### Patrón WebSocket

Un único goroutine en `main()` lee de `logChan` y distribuye a cada `wsClientConn.ch` (canal por cliente). Esto evita el problema de consumidor único.

### Flujo ROC (`rocHandler`)

1. **`ptr` / `full`**: FC03 en `ptr_addr` con `ptr_qty` registros → decodifica como float32 (CDAB) o int16 → `ptr_value`
2. **`hist` / `full`**: FC03 en `db_addr + uint16(ptr_value)` con Qty=1

---

## Comandos de desarrollo

```bash
make help          # Lista todos los targets con descripción
make build         # Compila ./modbus_client
make run           # Compila y ejecuta
make dev           # Ejecuta con race detector
make fmt           # go fmt ./...
make vet           # go vet ./...
make check         # fmt + vet
make test          # go test ./...
make deps          # go mod tidy
make clean         # Elimina el binario
make clean-logs    # Elimina logs/modbus_debug.log
make info          # Muestra versión, módulo y Go runtime
```

---

© 2026 EPM — Empresas Públicas de Medellín · Herramienta de Ingeniería Industrial
