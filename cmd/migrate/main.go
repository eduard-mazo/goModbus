// cmd/migrate/main.go — migración de station_history: popula dato1..dato10
// desde el campo hex existente usando el DBEndian de cada estación.
//
// Uso:
//
//	go run ./cmd/migrate/ [-db modbus.db] [-cfg config.yaml]
//	make migrate
//	make migrate DB=correcciones/modbus.db
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	"goModbus/internal/config"
	idb "goModbus/internal/db"
	"goModbus/internal/modbus"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := flag.String("db", "modbus.db", "SQLite database path")
	cfgPath := flag.String("cfg", "config.yaml", "YAML config path")
	flag.Parse()

	// Abre la BD — aplica la migración que añade dato1..dato10 si faltan.
	db, err := idb.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error abriendo DB: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Carga config → mapa task_key → DBEndian
	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error cargando config: %v\n", err)
		os.Exit(1)
	}
	endianMap := buildEndianMap(cfg)

	// Consulta todos los registros con hex no vacío
	rows, err := db.Query(
		`SELECT task_key, fecha, hora, hex FROM station_history WHERE hex <> '' ORDER BY task_key, fecha, hora`,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error consultando station_history: %v\n", err)
		os.Exit(1)
	}

	type row struct {
		taskKey, fecha, hora, hexStr string
	}
	var records []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.taskKey, &r.fecha, &r.hora, &r.hexStr); err != nil {
			rows.Close()
			fmt.Fprintf(os.Stderr, "error leyendo fila: %v\n", err)
			os.Exit(1)
		}
		records = append(records, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "error iterando rows: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Registros a migrar: %d\n", len(records))
	if len(records) == 0 {
		fmt.Println("Nada que migrar.")
		return
	}

	// Actualiza en una sola transacción
	tx, err := db.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error iniciando transacción: %v\n", err)
		os.Exit(1)
	}

	stmt, err := tx.Prepare(`
		UPDATE station_history
		SET dato1=?, dato2=?, dato3=?, dato4=?, dato5=?,
		    dato6=?, dato7=?, dato8=?, dato9=?, dato10=?
		WHERE task_key=? AND fecha=? AND hora=?`)
	if err != nil {
		tx.Rollback()
		fmt.Fprintf(os.Stderr, "error preparando UPDATE: %v\n", err)
		os.Exit(1)
	}
	defer stmt.Close()

	var updated, skipped, unknown int
	taskStats := map[string][2]int{} // task_key → [updated, skipped]

	for _, r := range records {
		endian, ok := endianMap[r.taskKey]
		if !ok {
			// task_key no encontrado en config (estación eliminada o renombrada)
			if unknown == 0 {
				fmt.Printf("  WARN task_key sin endian en config: %q\n", r.taskKey)
			}
			unknown++
			continue
		}

		data, err := hex.DecodeString(r.hexStr)
		if err != nil || len(data) == 0 {
			skipped++
			continue
		}

		modes := modbus.DecodeAllModes(data)
		var datos [10]float64
		for i := range datos {
			if i < len(modes) {
				datos[i] = float64(modes[i].Pick(endian))
			}
		}

		if _, err := stmt.Exec(
			datos[0], datos[1], datos[2], datos[3], datos[4],
			datos[5], datos[6], datos[7], datos[8], datos[9],
			r.taskKey, r.fecha, r.hora,
		); err != nil {
			tx.Rollback()
			fmt.Fprintf(os.Stderr, "error actualizando %s %s %s: %v\n", r.taskKey, r.fecha, r.hora, err)
			os.Exit(1)
		}
		updated++
		st := taskStats[r.taskKey]
		st[0]++
		taskStats[r.taskKey] = st
	}

	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "error en commit: %v\n", err)
		os.Exit(1)
	}

	// Resumen por estación
	fmt.Println()
	fmt.Printf("%-35s  %s\n", "Task key", "Registros migrados")
	fmt.Printf("%-35s  %s\n", "---", "---")
	for _, r := range records {
		// Imprime una línea por task_key (primera vez)
		if st := taskStats[r.taskKey]; st[1] == 0 {
			st[1] = 1 // marcar como impreso
			taskStats[r.taskKey] = st
			fmt.Printf("%-35s  %d\n", r.taskKey, st[0])
		}
	}
	fmt.Println()
	fmt.Printf("✓ Actualizados: %d  |  Saltados (hex inválido): %d  |  Sin config: %d\n",
		updated, skipped, unknown)
}

// buildEndianMap construye un mapa task_key → DBEndian desde la config.
func buildEndianMap(cfg *config.Config) map[string]modbus.Endianness {
	m := make(map[string]modbus.Endianness)
	for _, s := range cfg.Stations {
		dbEndian := s.DBEndian
		if len(s.Medidores) > 0 {
			for _, med := range s.Medidores {
				e := dbEndian
				if med.DBEndian != "" {
					e = med.DBEndian
				}
				key := fmt.Sprintf("%s / %s", s.Name, med.Name)
				m[key] = e
			}
		} else {
			m[s.Name] = dbEndian
		}
	}
	return m
}

