package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

func mask(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return strings.Repeat("*", len(value))
	}
	return value[:4] + "..." + value[len(value)-4:]
}

func randomToken(length int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var builder strings.Builder
	builder.Grow(length)
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			log.Fatal(err)
		}
		builder.WriteByte(alphabet[n.Int64()])
	}
	return builder.String()
}

func main() {
	_ = godotenv.Load(".env")

	dsn := os.Getenv("SQL_DSN")
	if dsn == "" {
		log.Fatal("SQL_DSN is empty")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatal(err)
	}

	if os.Getenv("OUTPUT_ROOT_TOKEN_ONLY") == "1" {
		var key string
		err := db.QueryRow(
			"SELECT `key` FROM tokens WHERE user_id = 1 AND status = 1 ORDER BY id DESC LIMIT 1",
		).Scan(&key)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(key)
		return
	}

	if os.Getenv("OUTPUT_ACTIVE_TOKEN_ONLY") == "1" {
		var key string
		err := db.QueryRow(
			"SELECT `key` FROM tokens WHERE status = 1 AND deleted_at IS NULL ORDER BY id DESC LIMIT 1",
		).Scan(&key)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(key)
		return
	}

	if os.Getenv("CREATE_ROOT_TOKEN_ONLY") == "1" {
		userID := 1
		if rawUserID := os.Getenv("TOKEN_USER_ID"); rawUserID != "" {
			fmt.Sscanf(rawUserID, "%d", &userID)
		}

		groupName := os.Getenv("TOKEN_GROUP")
		if groupName == "" {
			groupName = "default"
		}

		var key string
		err := db.QueryRow(
			"SELECT `key` FROM tokens WHERE user_id = ? AND status = 1 AND deleted_at IS NULL AND `group` = ? ORDER BY id DESC LIMIT 1",
			userID,
			groupName,
		).Scan(&key)
		if err == nil && key != "" {
			fmt.Print(key)
			return
		}
		if err != nil && err != sql.ErrNoRows {
			log.Fatal(err)
		}

		key = randomToken(48)
		now := time.Now().Unix()
		_, err = db.Exec(
			"INSERT INTO tokens (user_id, `key`, status, name, created_time, accessed_time, expired_time, remain_quota, unlimited_quota, model_limits_enabled, model_limits, allow_ips, used_quota, `group`, cross_group_retry) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			userID,
			key,
			1,
			fmt.Sprintf("yix_system_%d", userID),
			now,
			now,
			-1,
			0,
			true,
			false,
			"",
			"",
			0,
			groupName,
			false,
		)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Print(key)
		return
	}

	fmt.Println("Users:")
	userRows, err := db.Query(
		"SELECT id, username, role, status, quota, used_quota, COALESCE(`group`, '') " +
			"FROM users ORDER BY id ASC LIMIT 10",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer userRows.Close()

	for userRows.Next() {
		var (
			id        int
			username  string
			role      int
			status    int
			quota     int
			usedQuota int
			group     string
		)
		if err := userRows.Scan(&id, &username, &role, &status, &quota, &usedQuota, &group); err != nil {
			log.Fatal(err)
		}
		fmt.Printf("- id=%d username=%s role=%d status=%d quota=%d used=%d group=%s\n", id, username, role, status, quota, usedQuota, group)
	}

	fmt.Println("\nTokens:")
	tokenRows, err := db.Query(
		"SELECT id, user_id, name, status, `key`, expired_time, remain_quota, unlimited_quota, COALESCE(`group`, ''), deleted_at " +
			"FROM tokens ORDER BY id DESC LIMIT 20",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer tokenRows.Close()

	for tokenRows.Next() {
		var (
			id             int
			userID         int
			name           string
			status         int
			key            string
			expiredTime    int64
			remainQuota    int
			unlimitedQuota bool
			group          string
			deletedAt      sql.NullTime
		)
		if err := tokenRows.Scan(&id, &userID, &name, &status, &key, &expiredTime, &remainQuota, &unlimitedQuota, &group, &deletedAt); err != nil {
			log.Fatal(err)
		}
		deletedState := "null"
		if deletedAt.Valid {
			deletedState = deletedAt.Time.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("- id=%d user_id=%d name=%s status=%d key=%s expired=%d remain=%d unlimited=%t group=%s deleted_at=%s\n", id, userID, name, status, mask(key), expiredTime, remainQuota, unlimitedQuota, group, deletedState)
	}
}
