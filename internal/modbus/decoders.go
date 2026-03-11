package modbus

import (
	"fmt"
	"math"
	"time"
)

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

// Pick returns the float32 value for the given endianness string.
func (f *Float32Modes) Pick(endian Endianness) float32 {
	switch endian {
	case LittleEndian:
		return f.DCBA
	case WordSwapped:
		return f.CDAB
	case ByteSwapped:
		return f.BADC
	default:
		return f.ABCD
	}
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

// DecodeROCDateTime reads the first two Float32Modes entries as ROC date and time.
//
// ROC encodes date as a float whose integer value follows the MMDDYY pattern:
//
//	dateFloat = MM*10000 + DD*100 + YY   (e.g. 20315.0 = March 15, 2020+YY)
//
// Time is encoded as:
//
//	timeFloat = HH*100 + MM             (e.g. 1145.0 = 11:45)
//
// Returns (fecha "YYYY-MM-DD", hora "HH:MM", unix-seconds, ok).
// ok is false if the decoded values are out of plausible range.
func DecodeROCDateTime(modes []Float32Modes, endian Endianness) (fecha, hora string, ts int64, ok bool) {
	if len(modes) < 2 {
		return
	}
	dateVal := modes[0].Pick(endian)
	timeVal := modes[1].Pick(endian)

	dv := int(math.Round(float64(dateVal)))
	tv := int(math.Round(float64(timeVal)))

	month := dv / 10000
	day := (dv % 10000) / 100
	year := dv%100 + 2000
	hour := tv / 100
	minute := tv % 100

	if month < 1 || month > 12 || day < 1 || day > 31 ||
		year < 2000 || year > 2099 || hour > 23 || minute > 59 {
		return
	}

	t := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
	fecha = t.Format("2006-01-02")
	hora = fmt.Sprintf("%02d:%02d", hour, minute)
	ts = t.Unix()
	ok = true
	return
}

// DecodeBits unpacks bit values from coil response bytes
func DecodeBits(data []byte, qty uint16) []bool {
	bits := make([]bool, qty)
	for i := uint16(0); i < qty && int(i/8) < len(data); i++ {
		bits[i] = (data[i/8]>>(i%8))&1 == 1
	}
	return bits
}

// HourRecord represents one ROC circular-buffer record.
//
// The first two Float32Modes in Modes are date and time (ROC MMDDYY / HHMM encoding).
// Measurement signals start at Modes[2] and run through Modes[9] (8 channels).
//
// Fecha, Hora, and TS are derived from Modes[0]/Modes[1] and the station endianness.
type HourRecord struct {
	Ptr   uint16         `json:"ptr"`
	Hex   string         `json:"hex"`
	Modes []Float32Modes `json:"modes,omitempty"` // [0]=date float, [1]=time float, [2..9]=signals
	Valid bool           `json:"valid"`
	Fecha string         `json:"fecha,omitempty"` // "YYYY-MM-DD" decoded from Modes[0]
	Hora  string         `json:"hora,omitempty"`  // "HH:MM"      decoded from Modes[1]
	TS    int64          `json:"ts,omitempty"`    // unix timestamp (seconds)
}
