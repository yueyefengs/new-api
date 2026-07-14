package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultNewAPIDSN   = "root:toor0109@tcp(localhost:3306)/new_api?charset=utf8mb4&parseTime=true&loc=Local"
	targetChannelID    = 1
	targetImageModelID = "doubao-seedream-4-0-250828"
)

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func mustOpen(name, dsn string) *sql.DB {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(fmt.Errorf("open %s db: %w", name, err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Errorf("ping %s db: %w", name, err))
	}
	return db
}

func fetchString(db *sql.DB, query string, args ...any) string {
	var value string
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		panic(err)
	}
	return value
}

func fetchCount(db *sql.DB, query string, args ...any) int {
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		panic(err)
	}
	return count
}

func requireContains(label, value string, required ...string) {
	for _, item := range required {
		if !strings.Contains(value, item) {
			panic(fmt.Errorf("%s missing %q", label, item))
		}
	}
}

func main() {
	db := mustOpen("new_api", envOrDefault("NEW_API_VERIFY_DSN", defaultNewAPIDSN))
	defer db.Close()

	channelModels := fetchString(
		db,
		"SELECT COALESCE(models, '') FROM channels WHERE id = ?",
		targetChannelID,
	)
	requireContains("channels.models", channelModels, targetImageModelID)

	abilityCount := fetchCount(
		db,
		"SELECT COUNT(*) FROM abilities WHERE channel_id = ? AND model = ?",
		targetChannelID,
		targetImageModelID,
	)
	if abilityCount == 0 {
		panic(fmt.Errorf("expected abilities for channel=%d model=%s", targetChannelID, targetImageModelID))
	}

	modelRatioValue := fetchString(
		db,
		"SELECT value FROM options WHERE `key` = 'ModelRatio'",
	)
	requireContains("ModelRatio", modelRatioValue, targetImageModelID, ":0.5")

	fmt.Printf(
		"Verified YiX image gateway setup: channel=%d model=%s abilities=%d\n",
		targetChannelID,
		targetImageModelID,
		abilityCount,
	)
}
