package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	dsn := os.Getenv("SQL_DSN")
	if dsn == "" {
		log.Fatal("SQL_DSN is required")
	}
	if len(os.Args) < 2 {
		log.Fatal("usage: go run .codex-tmp/dbexec.go \"<sql>\"")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	result, err := db.Exec(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	rowsAffected, _ := result.RowsAffected()
	fmt.Printf("rows_affected=%d\n", rowsAffected)
}
