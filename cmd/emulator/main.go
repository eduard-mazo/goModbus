// cmd/emulator/main.go — Modbus TCP emulator for offline debugging.
//
// Reads config.yaml and a modbus.db SQLite file, then starts one TCP listener
// per unique device IP. Each listener responds to FC03 requests using the
// real frames stored in the database:
//
//   - Pointer register (addr = pointer_address, qty = data_registers_count):
//     encodes ref_ptr as float32/uint16 with the station's PtrEndian.
//   - Data record (addr = base_data_address, qty = ptr_index):
//     returns the 40-byte payload stored for that circular-buffer slot.
//
// Usage:
//
//	go run ./cmd/emulator/ [-db <path>] [-cfg <path>] [-port <base>]
//
// Then edit config.yaml to point each station IP → 127.0.0.1:<port>.
package main

import (
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"sort"
	"strings"
	"sync"

	"goModbus/internal/config"
	idb "goModbus/internal/db"
	"goModbus/internal/modbus"

	_ "modernc.org/sqlite"
)

// ─── Config & data model ──────────────────────────────────────────────────────

type taskSpec struct {
	key       string
	unitID    byte
	ptrAddr   uint16
	dbAddr    uint16
	ptrEndian modbus.Endianness
	drc       uint16 // 1=uint16 pointer, 2=float32 pointer
	refPtr    int
	records   map[int][]byte // ptr → 40-byte payload
}

type endpoint struct {
	ip    string
	tasks []*taskSpec
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	dbPath := flag.String("db", "correcciones/modbus.db", "SQLite database path")
	cfgPath := flag.String("cfg", "config.yaml", "YAML config path")
	basePort := flag.Int("port", 15000, "Base TCP listen port (IP[0] → base+1, IP[1] → base+2, …)")
	flag.Parse()

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config: %v\n", err)
		os.Exit(1)
	}

	db, err := idb.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	endpoints := buildEndpoints(cfg, db)
	printBanner(*dbPath, endpoints, *basePort)

	var wg sync.WaitGroup
	port := *basePort
	for _, ep := range endpoints {
		port++
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ep := ep
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := serve(addr, ep); err != nil {
				fmt.Fprintf(os.Stderr, "listener %s: %v\n", addr, err)
			}
		}()
	}
	wg.Wait()
}

// ─── Build endpoint map ───────────────────────────────────────────────────────

func buildEndpoints(cfg *config.Config, db *sql.DB) []*endpoint {
	ipMap := map[string]*endpoint{}
	var ipOrder []string

	for _, s := range cfg.Stations {
		drc := s.DataRegistersCount
		if drc == 0 {
			drc = 1
		}

		if len(s.Medidores) > 0 {
			for _, m := range s.Medidores {
				pe := s.PtrEndian
				if m.PtrEndian != "" {
					pe = m.PtrEndian
				}
				addTask(db, ipMap, &ipOrder, s.IP, &taskSpec{
					key:       fmt.Sprintf("%s / %s", s.Name, m.Name),
					unitID:    s.ID,
					ptrAddr:   m.PointerAddress,
					dbAddr:    m.DBAddress,
					ptrEndian: pe,
					drc:       drc,
				})
			}
		} else {
			addTask(db, ipMap, &ipOrder, s.IP, &taskSpec{
				key:       s.Name,
				unitID:    s.ID,
				ptrAddr:   s.PointerAddress,
				dbAddr:    s.DBAddress,
				ptrEndian: s.PtrEndian,
				drc:       drc,
			})
		}
	}

	out := make([]*endpoint, 0, len(ipOrder))
	for _, ip := range ipOrder {
		out = append(out, ipMap[ip])
	}
	return out
}

func addTask(db *sql.DB, m map[string]*endpoint, order *[]string, ip string, t *taskSpec) {
	if _, ok := m[ip]; !ok {
		m[ip] = &endpoint{ip: ip}
		*order = append(*order, ip)
	}
	t.records = loadRecords(db, t.key)
	t.refPtr = loadRefPtr(db, t.key)
	m[ip].tasks = append(m[ip].tasks, t)
}

func loadRecords(db *sql.DB, taskKey string) map[int][]byte {
	recs, err := idb.GetTaskRecords(db, taskKey)
	if err != nil {
		return map[int][]byte{}
	}
	out := make(map[int][]byte, len(recs))
	for ptr, r := range recs {
		if r.Valid && r.Hex != "" {
			if b, err := hex.DecodeString(r.Hex); err == nil {
				out[ptr] = b
			}
		}
	}
	return out
}

func loadRefPtr(db *sql.DB, taskKey string) int {
	meta, err := idb.GetTaskMeta(db, taskKey)
	if err != nil || meta == nil {
		return -1
	}
	return meta.RefPtr
}

// ─── TCP server ───────────────────────────────────────────────────────────────

func serve(addr string, ep *endpoint) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handleConn(conn, ep)
	}
}

func handleConn(conn net.Conn, ep *endpoint) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	fmt.Printf("[%s ← %s] conexión\n", ep.ip, remote)

	hdr := make([]byte, 6)
	body := make([]byte, 512)

	for {
		// Read MBAP header (6 bytes)
		if _, err := io.ReadFull(conn, hdr); err != nil {
			if err != io.EOF {
				fmt.Printf("[%s] RX error: %v\n", ep.ip, err)
			}
			return
		}
		txID := binary.BigEndian.Uint16(hdr[0:2])
		length := int(binary.BigEndian.Uint16(hdr[4:6]))
		if length < 2 || length > len(body) {
			fmt.Printf("[%s] MBAP length inválido: %d\n", ep.ip, length)
			return
		}
		if _, err := io.ReadFull(conn, body[:length]); err != nil {
			fmt.Printf("[%s] RX PDU error: %v\n", ep.ip, err)
			return
		}

		full := append(hdr, body[:length]...)
		unitID := body[0]
		fc := body[1]
		pdu := body[2:length]

		fmt.Printf("[%s] → TX[%d]: %X\n", ep.ip, len(full), full)

		resp := dispatch(txID, unitID, fc, pdu, ep)
		fmt.Printf("[%s] ← RX[%d]: %X\n", ep.ip, len(resp), resp)

		if _, err := conn.Write(resp); err != nil {
			fmt.Printf("[%s] TX error: %v\n", ep.ip, err)
			return
		}
	}
}

// ─── Request dispatch ─────────────────────────────────────────────────────────

func dispatch(txID uint16, unitID, fc byte, pdu []byte, ep *endpoint) []byte {
	if fc != 0x03 {
		return exception(txID, unitID, fc, 0x01) // Illegal Function
	}
	if len(pdu) < 4 {
		return exception(txID, unitID, fc, 0x03) // Illegal Data Value
	}

	addr := binary.BigEndian.Uint16(pdu[0:2])
	qty := binary.BigEndian.Uint16(pdu[2:4])

	// Match pointer request first (addr == ptrAddr AND qty == drc)
	for _, t := range ep.tasks {
		if addr == t.ptrAddr && qty == t.drc {
			data := encodePtrValue(t.refPtr, t.drc, t.ptrEndian)
			fmt.Printf("  ptrAddr=%-5d drc=%d → ptr=%d [%X] (%s)\n",
				addr, qty, t.refPtr, data, t.key)
			return fc03Response(txID, unitID, data)
		}
	}

	// Match data record request (addr == dbAddr, qty = circular-buffer index)
	for _, t := range ep.tasks {
		if addr == t.dbAddr {
			ptr := int(qty)
			data, ok := t.records[ptr]
			if !ok {
				fmt.Printf("  dbAddr=%-5d ptr=%-4d → no encontrado (%s) → exc 0x02\n",
					addr, ptr, t.key)
				return exception(txID, unitID, fc, 0x02)
			}
			fmt.Printf("  dbAddr=%-5d ptr=%-4d → %d bytes [%.20s…] (%s)\n",
				addr, ptr, len(data), fmt.Sprintf("%X", data), t.key)
			return fc03Response(txID, unitID, data)
		}
	}

	fmt.Printf("  addr=%-5d qty=%-4d → sin coincidencia → exc 0x02\n", addr, qty)
	return exception(txID, unitID, fc, 0x02)
}

// encodePtrValue encodes the pointer index as float32 (drc=2) or uint16 (drc=1).
func encodePtrValue(ptr int, drc uint16, endian modbus.Endianness) []byte {
	if drc >= 2 {
		bits := math.Float32bits(float32(ptr))
		var b [4]byte
		switch endian {
		case modbus.LittleEndian: // dcba: b[3]<<24|b[2]<<16|b[1]<<8|b[0] = bits
			b[3] = byte(bits >> 24)
			b[2] = byte(bits >> 16)
			b[1] = byte(bits >> 8)
			b[0] = byte(bits)
		case modbus.WordSwapped: // cdab: b[2]<<24|b[3]<<16|b[0]<<8|b[1] = bits
			b[2] = byte(bits >> 24)
			b[3] = byte(bits >> 16)
			b[0] = byte(bits >> 8)
			b[1] = byte(bits)
		case modbus.ByteSwapped: // badc: b[1]<<24|b[0]<<16|b[3]<<8|b[2] = bits
			b[1] = byte(bits >> 24)
			b[0] = byte(bits >> 16)
			b[3] = byte(bits >> 8)
			b[2] = byte(bits)
		default: // abcd: b[0]<<24|b[1]<<16|b[2]<<8|b[3] = bits
			b[0] = byte(bits >> 24)
			b[1] = byte(bits >> 16)
			b[2] = byte(bits >> 8)
			b[3] = byte(bits)
		}
		return b[:]
	}
	// uint16 big-endian
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(ptr))
	return b
}

// ─── Modbus response builders ─────────────────────────────────────────────────

// fc03Response builds a valid Modbus TCP FC03 response frame.
func fc03Response(txID uint16, unitID byte, data []byte) []byte {
	// MBAP(6) + unitID(1) + FC(1) + byteCount(1) + data
	resp := make([]byte, 9+len(data))
	binary.BigEndian.PutUint16(resp[0:], txID)
	binary.BigEndian.PutUint16(resp[2:], 0)                    // Protocol ID
	binary.BigEndian.PutUint16(resp[4:], uint16(3+len(data))) // Length = unitID+FC+byteCount+data
	resp[6] = unitID
	resp[7] = 0x03
	resp[8] = byte(len(data))
	copy(resp[9:], data)
	return resp
}

// exception builds a Modbus TCP exception response.
func exception(txID uint16, unitID, fc, code byte) []byte {
	resp := make([]byte, 9)
	binary.BigEndian.PutUint16(resp[0:], txID)
	binary.BigEndian.PutUint16(resp[2:], 0)
	binary.BigEndian.PutUint16(resp[4:], 3) // unitID(1) + FC|0x80(1) + code(1)
	resp[6] = unitID
	resp[7] = fc | 0x80
	resp[8] = code
	return resp
}

// ─── Banner ───────────────────────────────────────────────────────────────────

func printBanner(dbPath string, endpoints []*endpoint, basePort int) {
	w := 70
	line := strings.Repeat("─", w-2)
	fmt.Printf("┌%s┐\n", line)
	center := func(s string) {
		pad := w - 2 - len(s)
		l, r := pad/2, pad-pad/2
		fmt.Printf("│%s%s%s│\n", strings.Repeat(" ", l), s, strings.Repeat(" ", r))
	}
	center("ROC Modbus Emulator — depuración offline")
	fmt.Printf("├%s┤\n", line)
	center(fmt.Sprintf("DB: %s", dbPath))
	fmt.Printf("├%s┤\n", line)

	// Header row
	fmt.Printf("│  %-5s  %-20s  %-15s  %-20s│\n",
		"Puerto", "IP real", "Tareas/recs", "Task keys")
	fmt.Printf("├%s┤\n", line)

	port := basePort
	for _, ep := range endpoints {
		port++
		// Summarize tasks
		totalRecs := 0
		var keys []string
		for _, t := range ep.tasks {
			totalRecs += len(t.records)
			shortKey := t.key
			if idx := strings.Index(t.key, " / "); idx >= 0 {
				shortKey = t.key[idx+3:] // "M1", "M2", ...
			}
			keys = append(keys, shortKey)
		}
		sort.Strings(keys)
		keySummary := strings.Join(keys, ",")
		if len(keySummary) > 20 {
			keySummary = keySummary[:17] + "..."
		}
		taskSummary := fmt.Sprintf("%dt/%drec", len(ep.tasks), totalRecs)

		fmt.Printf("│  %-5d  %-20s  %-15s  %-20s│\n",
			port, ep.ip, taskSummary, keySummary)
	}

	fmt.Printf("├%s┤\n", line)
	center("Edita config.yaml: ip → 127.0.0.1,  port → puerto")
	fmt.Printf("└%s┘\n", line)
	fmt.Println()
}
