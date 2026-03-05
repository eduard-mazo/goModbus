package modbus

import "math"

// Float32Modes holds one float32 value decoded under all four endianness conventions.
type Float32Modes struct {
	ABCD float32 `json:"abcd"` // Big-Endian
	DCBA float32 `json:"dcba"` // Little-Endian
	CDAB float32 `json:"cdab"` // Word-Swap (ROC default)
	BADC float32 `json:"badc"` // Byte-Swap
}

// Sanitize replaces NaN or Inf values with 0 to prevent JSON marshal errors.
func (f *Float32Modes) Sanitize() {
	f.ABCD = SanitizeFloat(f.ABCD)
	f.DCBA = SanitizeFloat(f.DCBA)
	f.CDAB = SanitizeFloat(f.CDAB)
	f.BADC = SanitizeFloat(f.BADC)
}

func SanitizeFloat(v float32) float32 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return v
}

// DecodeAllModes decodes each 4-byte group in data under all four endianness modes.
func DecodeAllModes(data []byte) []Float32Modes {
	out := make([]Float32Modes, 0, len(data)/4)
	for i := 0; i+4 <= len(data); i += 4 {
		b := data[i : i+4]
		out = append(out, Float32Modes{
			ABCD: math.Float32frombits(uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])),
			DCBA: math.Float32frombits(uint32(b[3])<<24 | uint32(b[2])<<16 | uint32(b[1])<<8 | uint32(b[0])),
			CDAB: math.Float32frombits(uint32(b[2])<<24 | uint32(b[3])<<16 | uint32(b[0])<<8 | uint32(b[1])),
			BADC: math.Float32frombits(uint32(b[1])<<24 | uint32(b[0])<<16 | uint32(b[3])<<8 | uint32(b[2])),
		})
	}
	return out
}

// DecodeBits unpacks bit values from coil response bytes
func DecodeBits(data []byte, qty uint16) []bool {
	bits := make([]bool, qty)
	for i := uint16(0); i < qty && int(i/8) < len(data); i++ {
		bits[i] = (data[i/8]>>(i%8))&1 == 1
	}
	return bits
}

// HourRecord represents one ROC circular-buffer slot (840 total per station).
type HourRecord struct {
	Hour  int           `json:"hour"`
	Ptr   uint16        `json:"ptr"`
	Hex   string        `json:"hex"`
	Value float32       `json:"value"`         // first float in db_endian (bytes 4+)
	Modes []Float32Modes `json:"modes,omitempty"` // all 4 endianness decodings (bytes 4+)
	Valid bool          `json:"valid"`
}
