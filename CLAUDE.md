# ROC Modbus Expert: Technical Guide & Style

## 🛠 Comandos de Desarrollo

- **Compilar (Windows):** `make build` (Genera `modbus_client.exe`)
- **Modo Desarrollo:** `make dev` (Lee archivos `index.html` y `/static` desde disco en tiempo real)
- **Limpiar:** `make clean`
- **Tidying:** `go mod tidy`

## 🏗 Arquitectura de Sincronización Paralela

- **POST /api/stations/full-sync**:
    - `Stations` (array strings): Filtra estaciones del `config.yaml`.
    - **Algoritmo**: Lanza 1 goroutine por estación. Cada estación usa un **Worker Pool interno de 2 trabajadores** para bajar los 840 registros de forma concurrente, evitando la saturación del puerto TCP del equipo esclavo.
- **POST /api/stations/partial-sync**:
    - Reintenta solo los `pointers` (array uint16) específicos que fallaron, recibiendo IP/Puerto/ID/Endian para precisión quirúrgica.

## ✒️ Guía de Estilo

- **Go**: Seguir `go fmt`. Nombres de variables camelCase, excepto constantes (PascalCase o UPPER_CASE según contexto).
- **Frontend**: 
    - Alpine.js para la reactividad.
    - Mantener `index.html` como SPA (Single Page Application).
    - Los estilos se definen en bloques `<style>` y colores se ajustan para contrastes profesionales (#0f172a para cabeceras, #f8fafc para fondo).
- **Logs**: Usar `broadcastLog` para enviar tramas TX/RX a la terminal web.
- **Seguridad**: Sanitizar siempre los floats (`NaN` e `Inf`) a `0` antes de la serialización JSON para evitar errores de parseo en el navegador.

## 📂 Estructura de Archivos
- `main.go`: Servidor Gin y WebSockets.
- `handlers.go`: Lógica de API REST (Query, ROC, Sync).
- `modbus.go`: Cliente Modbus TCP y decodificadores Endian.
- `config.go`: Persistencia de configuración YAML.
- `index.html`: UI Reactiva y lógica del cliente Alpine.js.
