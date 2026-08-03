package main

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	dbPath := "data/agent.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Printf("❌ Database file not found at: %s\n", dbPath)
		fmt.Println("Start the agent service or UI first to generate database records!")
		return
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Printf("❌ Error opening database: %v\n", err)
		return
	}
	defer db.Close()

	fmt.Println("=========================================================================================")
	fmt.Printf(" 🗄️  SQLITE DATABASE VIEWER - UNIFIED ACTIVITY & EVENTS - [%s]\n", dbPath)
	fmt.Println("=========================================================================================")

	// 1. Unified App & Web Activity Table
	fmt.Println("\n💻 UNIFIED ACTIVITIES TABLE (activities):")
	unifiedQuery := `
		SELECT id, category, app_name, window_title || CASE WHEN domain != '' THEN ' [' || domain || ']' ELSE '' END, pid, duration_sec, datetime(created_at, 'localtime')
		FROM activities ORDER BY id DESC LIMIT 15`

	printTable(db, unifiedQuery, []string{"ID", "CATEGORY", "APP / BROWSER", "TITLE / DOMAIN", "PID", "DURATION(s)", "CREATED AT"})


	// 2. Input Activities
	fmt.Println("\n⌨️ INPUT ACTIVITIES (input_activities):")
	printTable(db, "SELECT id, keyboard_count, mouse_clicks, ROUND(mouse_move_dist,1), idle_time_sec, datetime(created_at, 'localtime') FROM input_activities ORDER BY id DESC LIMIT 10",
		[]string{"ID", "KEYPRESSES", "CLICKS", "DISTANCE(px)", "IDLE(s)", "CREATED AT"})

	// 3. Screenshots
	fmt.Println("\n📸 SCREENSHOTS (screenshots):")
	printTable(db, "SELECT id, file_path, file_size, width || 'x' || height, datetime(captured_at, 'localtime'), sync_status FROM screenshots ORDER BY id DESC LIMIT 10",
		[]string{"ID", "FILE PATH", "SIZE(bytes)", "RESOLUTION", "CAPTURED AT", "SYNC STATUS"})

	// 4. Sync Queue
	fmt.Println("\n🔄 SYNC QUEUE (sync_queue):")
	printTable(db, "SELECT id, payload_type, SUBSTR(payload_json, 1, 40), retry_count, status, datetime(created_at, 'localtime') FROM sync_queue ORDER BY id DESC LIMIT 10",
		[]string{"ID", "TYPE", "PAYLOAD PREVIEW", "RETRIES", "STATUS", "CREATED AT"})

	fmt.Println("\n=========================================================================================")
}

func printTable(db *sql.DB, query string, headers []string) {
	rows, err := db.Query(query)
	if err != nil {
		fmt.Printf("   (Error executing query: %v)\n", err)
		return
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)

	// Header line
	for i, h := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)

	// Divider
	for i := range headers {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, "--------------------")
	}
	fmt.Fprintln(w)

	count := 0
	cols, _ := rows.Columns()
	vals := make([]interface{}, len(cols))
	valPtrs := make([]interface{}, len(cols))
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	for rows.Next() {
		count++
		if err := rows.Scan(valPtrs...); err == nil {
			for i, v := range vals {
				if i > 0 {
					fmt.Fprint(w, "\t")
				}
				if b, ok := v.([]byte); ok {
					fmt.Fprint(w, string(b))
				} else if t, ok := v.(time.Time); ok {
					fmt.Fprint(w, t.Format("15:04:05"))
				} else if v != nil {
					fmt.Fprint(w, v)
				} else {
					fmt.Fprint(w, "-")
				}
			}
			fmt.Fprintln(w)
		}
	}

	if count == 0 {
		fmt.Println("   (No records found)")
	} else {
		w.Flush()
	}
}
