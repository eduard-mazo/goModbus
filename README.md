# ROC Modbus Expert v3.0 | EPM

Una herramienta profesional para la inspección, diagnóstico y sincronización masiva de dispositivos compatibles con Modbus TCP, optimizada para equipos Emerson ROC.

## 🚀 Características Principales

- **Consulta Modbus Genérica:** Soporte para Function Codes 01, 02, 03, 04, 05, 06, 15 y 16.
- **ROC Expert Mode:** Flujo de trabajo especializado para leer punteros de histórico y bloques de datos ROC.
- **Sincronización Masiva Paralela:** 
    - Descarga de los 840 registros del buffer circular de múltiples estaciones simultáneamente.
    - Implementación de **Worker Pools** (2 workers por estación) para optimizar tiempos de respuesta.
- **Gestión de Errores y Reintentos:**
    - Identificación visual de registros fallidos.
    - Sistema de **Reintento Parcial** que solo solicita los índices con error, optimizando el tráfico de red.
- **Decodificación Multi-Endian:** Visualización reactiva en tiempo real para ABCD (Big-Endian), DCBA (Little-Endian), CDAB (Word-Swap) y BADC (Byte-Swap).
- **Terminal de Diagnóstico:** Log detallado de tramas hexadecimales (TX/RX) y tiempos de respuesta (RTT).

## 🛠 Arquitectura Técnica

- **Backend:** Go 1.21+ con [Gin Gonic](https://github.com/gin-gonic/gin) para la API REST.
- **Frontend:** Alpine.js para la reactividad, Tailwind CSS para el estilizado y una arquitectura SPA (Single Page Application).
- **Comunicación:** WebSockets para streaming de logs en tiempo real y REST para comandos.
- **Portabilidad:** Binario único autoejecutable con archivos frontend embebidos mediante `go:embed`.

## 📋 Requisitos

- **Go** (para compilación).
- **Make** (opcional, para automatización).
- Acceso por red a los dispositivos Modbus TCP (Puerto 502 por defecto).

## 📦 Instalación y Uso

1. **Clonar el repositorio:**
   ```bash
   git clone <repo-url>
   cd goModbus
   ```

2. **Configurar estaciones:**
   Edita `config.yaml` para añadir tus dispositivos preconfigurados.

3. **Compilar:**
   ```bash
   make build
   ```

4. **Ejecutar:**
   ```bash
   ./modbus_client.exe
   ```
   Accede vía navegador a `http://localhost:8081`.

## ⚙️ Configuración (config.yaml)

```yaml
stations:
  - name: "Estación Ejemplo"
    ip: "10.155.150.201"
    port: 502
    id: 10
    endian: "cdab"
    pointer_address: 10000
    base_data_address: 700
```

---
Desarrollado para el ecosistema de infraestructura crítica de **EPM**.
