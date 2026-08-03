package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func main() {
	appData := os.Getenv("APPDATA")
	dbPath := filepath.Join(appData, "MonitoringAgent", "data", "agent.db")
	fmt.Println("Reading DB at:", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Println("Error opening DB:", err)
		return
	}
	defer db.Close()

	rows, err := db.Query("SELECT id, payload_type, payload_json, status, last_error FROM sync_queue ORDER BY id DESC LIMIT 10")
	if err != nil {
		fmt.Println("Error querying queue:", err)
		return
	}
	defer rows.Close()

	fmt.Println("\n--- APPDATA SYNC QUEUE ---")
	for rows.Next() {
		var id int64
		var pType, payloadJson, status, lastErr string
		var errNull sql.NullString
		rows.Scan(&id, &pType, &payloadJson, &status, &errNull)
		if errNull.Valid {
			lastErr = errNull.String
		}
		fmt.Printf("ID: %d | Type: %s | Payload: %s | Status: %s | Error: %s\n", id, pType, payloadJson, status, lastErr)
	}
}
