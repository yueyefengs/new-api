package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := os.Getenv("SQL_DSN")
	if dsn == "" {
		log.Fatal("SQL_DSN is required")
	}
	if len(os.Args) < 4 {
		log.Fatal("usage: go run .codex-tmp/set_option_json_key.go <option_key> <json_key> <float_value>")
	}

	optionKey := os.Args[1]
	jsonKey := os.Args[2]
	floatValue, err := strconv.ParseFloat(os.Args[3], 64)
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var raw string
	if err := db.QueryRow("SELECT value FROM options WHERE `key` = ?", optionKey).Scan(&raw); err != nil {
		log.Fatal(err)
	}

	payload := map[string]float64{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			log.Fatal(err)
		}
	}

	payload[jsonKey] = floatValue

	encoded, err := json.Marshal(payload)
	if err != nil {
		log.Fatal(err)
	}

	result, err := db.Exec("UPDATE options SET value = ? WHERE `key` = ?", string(encoded), optionKey)
	if err != nil {
		log.Fatal(err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("rows_affected=%d\n", rowsAffected)
}
