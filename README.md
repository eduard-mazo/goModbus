# Modbus ROC Master Expert

Una herramienta profesional de ingeniería para la comunicación, diagnóstico y monitoreo de controladores ROC (Remote Operations Controllers) mediante el protocolo Modbus TCP.

## 🚀 Características Principales

- **Flujo ROC Nativo**: Implementación exacta del protocolo de lectura histórica (Lectura de puntero seguida de consulta a base histórica con Qty=1).
- **Interfaz Reactiva Moderna**: UI diseñada con estética industrial, paleta de colores pastel para reducir fatiga visual y actualización en tiempo real de datos interpretados.
- **Interpretación Inteligente**: 
  - Decodificación automática de registros históricos ROC (Fecha `m/dd/yy`, Hora Militar `HH:MM` y Canales Análogos).
  - Soporte para múltiples formatos de *Endianness* (Word Swapped, Little Endian, Big Endian, Byte Swapped).
- **Diagnóstico Avanzado**:
  - Inyector de tramas RAW con decodificador en vivo.
  - Medición de latencia de respuesta (milisegundos).
  - Logs persistentes en archivo (`logs/modbus_debug.log`) y en consola web.
- **Standalone**: Ejecutable autónomo sin dependencias externas ni necesidad de internet.

## 📋 Requisitos

- Go 1.24+ (para compilar desde el código fuente)
- Navegador web moderno

## 🛠️ Instalación y Uso

1. **Compilar el proyecto**:
   ```bash
   make build
   ```
2. **Configurar estaciones**:
   Edita el archivo `config.yaml` para añadir tus dispositivos (ver sección [Configuración](#configuración)).
3. **Ejecutar**:
   ```bash
   ./modbus_client.exe
   ```
4. **Acceder a la Interfaz**:
   Abre tu navegador en `http://localhost:8080`.

## ⚙️ Configuración (`config.yaml`)

El sistema permite configurar cada estación con parámetros específicos de comunicación y direccionamiento:

```yaml
stations:
  - name: "Estación 1"
    ip: "10.155.150.201"
    port: 502
    id: 10
    pointer_address: 10000
    data_registers_count: 2 # Qty para leer el puntero
    pointer_endian: "cdab"
    db_address: 700
    db_endian: "cdab"
```

### Opciones de Endianness:
- `cdab`: Word Swapped (Estándar en la mayoría de ROCs)
- `dcba`: Little Endian
- `abcd`: Big Endian
- `badc`: Byte Swapped

## 🔍 Flujo de Operación ROC

1. **Lectura de Puntero**: El sistema consulta la dirección configurada para obtener el índice actual del histórico.
2. **Lectura Histórica**: Realiza una petición a `BaseAddress + Puntero` con una cantidad (`Qty`) de 1.
3. **Procesamiento**: La ROC responde con la trama completa del registro histórico, que el sistema desglosa automáticamente.

## 📂 Estructura del Proyecto

- `main.go`: Servidor API y orquestador de lógica.
- `utils.go`: Cliente Modbus, motores de decodificación y sistema de logging.
- `index.html`: Interfaz web reactiva standalone.
- `config.yaml`: Archivo de persistencia para estaciones.
- `logs/`: Directorio donde se almacenan los diagnósticos detallados.

---
© 2026 Modbus ROC Master Expert | Herramienta de Ingeniería.
