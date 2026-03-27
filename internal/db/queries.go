package db

import (
	"database/sql"
	"time"
)

// AppendQueryHistory records one generic Modbus query for auditing.
// Called from the query handler after each successful FC execution.
func AppendQueryHistory(db *sql.DB, h QueryHistory) error {
	_, err := db.Exec(
		`INSERT INTO query_history (queried_at, host, port, unit_id, fc, address, quantity, result_hex)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now(), h.Host, h.Port, h.UnitID, h.FC, h.Address, h.Quantity, h.ResultHex,
	)
	return err
}
