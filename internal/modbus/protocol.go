package modbus

import "encoding/binary"

// Endianness identifiers (Modbus byte/word order)
type Endianness string

const (
	BigEndian    Endianness = "abcd" // Standard Big-Endian
	LittleEndian Endianness = "dcba" // Full Little-Endian
	WordSwapped  Endianness = "cdab" // Word-Swapped (common in ROC devices)
	ByteSwapped  Endianness = "badc" // Byte-Swapped
)

// Modbus Function Codes
const (
	FCReadCoils            byte = 0x01
	FCReadDiscreteInputs   byte = 0x02
	FCReadHoldingRegisters byte = 0x03
	FCReadInputRegisters   byte = 0x04
	FCWriteSingleCoil      byte = 0x05
	FCWriteSingleRegister  byte = 0x06
	FCWriteMultipleCoils   byte = 0x0F
	FCWriteMultipleRegs    byte = 0x10
)

// ModbusExceptionDesc maps exception codes to human-readable descriptions
var ModbusExceptionDesc = map[byte]string{
	0x01: "Función no soportada",
	0x02: "Dirección fuera de rango",
	0x03: "Valor de dato incorrecto",
	0x04: "Fallo en dispositivo esclavo",
	0x05: "Confirmación (procesando)",
	0x06: "Esclavo ocupado",
	0x08: "Error de paridad en memoria",
	0x0A: "Gateway - ruta no disponible",
	0x0B: "Gateway - dispositivo no responde",
}

// buildPDU constructs the PDU (FC + data) for the given function code
func buildPDU(fc byte, addr, qty uint16, data []byte) []byte {
	switch fc {
	case FCReadCoils, FCReadDiscreteInputs, FCReadHoldingRegisters, FCReadInputRegisters:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		return pdu

	case FCWriteSingleCoil:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		if len(data) > 0 && data[0] != 0 {
			pdu[3], pdu[4] = 0xFF, 0x00 // ON
		} // else 0x00 0x00 = OFF
		return pdu

	case FCWriteSingleRegister:
		pdu := make([]byte, 5)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		if len(data) >= 2 {
			pdu[3], pdu[4] = data[0], data[1]
		}
		return pdu

	case FCWriteMultipleCoils:
		byteCount := (qty + 7) / 8
		pdu := make([]byte, 6+byteCount)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		pdu[5] = byte(byteCount)
		copy(pdu[6:], data)
		return pdu

	case FCWriteMultipleRegs:
		byteCount := qty * 2
		pdu := make([]byte, 6+byteCount)
		pdu[0] = fc
		binary.BigEndian.PutUint16(pdu[1:], addr)
		binary.BigEndian.PutUint16(pdu[3:], qty)
		pdu[5] = byte(byteCount)
		copy(pdu[6:], data)
		return pdu
	}
	return nil
}

// buildMBAP builds the full Modbus TCP Application Data Unit
func buildMBAP(txID uint16, unitID byte, pdu []byte) []byte {
	req := make([]byte, 7+len(pdu))
	binary.BigEndian.PutUint16(req[0:], txID)               // Transaction ID
	binary.BigEndian.PutUint16(req[2:], 0)                  // Protocol ID = 0
	binary.BigEndian.PutUint16(req[4:], uint16(1+len(pdu))) // Length = UnitID(1) + PDU
	req[6] = unitID
	copy(req[7:], pdu)
	return req
}
