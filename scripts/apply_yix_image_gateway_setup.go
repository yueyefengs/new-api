package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultNewAPIDSN     = "root:toor0109@tcp(localhost:3306)/new_api?charset=utf8mb4&parseTime=true&loc=Local"
	targetChannelID      = 1
	targetImageModelID   = "doubao-seedream-4-0-250828"
	referenceImageModel  = "gpt-image-2-all"
	targetModelRatio     = 0.5
	defaultChannelStatus = 1
)

type abilitySeed struct {
	Group    string
	Enabled  bool
	Priority int64
	Weight   uint64
	Tag      sql.NullString
}

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

func splitCSVUnique(raw string) []string {
	values := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func appendCSVUnique(raw string, value string) string {
	items := splitCSVUnique(raw)
	target := strings.TrimSpace(value)
	if target == "" {
		return strings.Join(items, ",")
	}
	for _, item := range items {
		if item == target {
			return strings.Join(items, ",")
		}
	}
	return strings.Join(append(items, target), ",")
}

func loadTargetAbilities(tx *sql.Tx, channelID int) ([]abilitySeed, error) {
	rows, err := tx.Query(
		""+
			"SELECT\n"+
			"  groups.group_name,\n"+
			"  COALESCE(ref.enabled, CASE WHEN channel_meta.status = ? THEN 1 ELSE 0 END) AS enabled,\n"+
			"  COALESCE(ref.priority, channel_meta.priority) AS priority,\n"+
			"  COALESCE(ref.weight, channel_meta.weight) AS weight,\n"+
			"  COALESCE(ref.tag, channel_meta.tag) AS tag\n"+
			"FROM (\n"+
			"  SELECT DISTINCT abilities.`group` AS group_name\n"+
			"  FROM abilities\n"+
			"  WHERE abilities.channel_id = ?\n"+
			") AS groups\n"+
			"JOIN (\n"+
			"  SELECT\n"+
			"    channels.id,\n"+
			"    COALESCE(channels.status, ?) AS status,\n"+
			"    COALESCE(channels.priority, 0) AS priority,\n"+
			"    COALESCE(channels.weight, 0) AS weight,\n"+
			"    channels.tag AS tag\n"+
			"  FROM channels\n"+
			"  WHERE channels.id = ?\n"+
			") AS channel_meta\n"+
			"  ON channel_meta.id = ?\n"+
			"LEFT JOIN abilities AS ref\n"+
			"  ON ref.channel_id = channel_meta.id\n"+
			" AND ref.`group` = groups.group_name\n"+
			" AND ref.model = ?\n"+
			"ORDER BY groups.group_name",
		defaultChannelStatus,
		channelID,
		defaultChannelStatus,
		channelID,
		channelID,
		referenceImageModel,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seeds := make([]abilitySeed, 0)
	for rows.Next() {
		var seed abilitySeed
		if err := rows.Scan(&seed.Group, &seed.Enabled, &seed.Priority, &seed.Weight, &seed.Tag); err != nil {
			return nil, err
		}
		seeds = append(seeds, seed)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return seeds, nil
}

func loadModelRatios(tx *sql.Tx) (map[string]any, error) {
	var raw sql.NullString
	err := tx.QueryRow("SELECT value FROM options WHERE `key` = 'ModelRatio' LIMIT 1").Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	ratios := make(map[string]any)
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return ratios, nil
	}

	if err := json.Unmarshal([]byte(raw.String), &ratios); err != nil {
		return nil, fmt.Errorf("parse ModelRatio JSON: %w", err)
	}
	return ratios, nil
}

func main() {
	db := mustOpen("new_api", envOrDefault("NEW_API_SETUP_DSN", defaultNewAPIDSN))
	defer db.Close()

	tx, err := db.Begin()
	if err != nil {
		panic(err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var currentModels string
	if scanErr := tx.QueryRow("SELECT COALESCE(models, '') FROM channels WHERE id = ?", targetChannelID).Scan(&currentModels); scanErr != nil {
		err = fmt.Errorf("load channel %d models: %w", targetChannelID, scanErr)
		panic(err)
	}

	nextModels := appendCSVUnique(currentModels, targetImageModelID)
	if _, execErr := tx.Exec("UPDATE channels SET models = ? WHERE id = ?", nextModels, targetChannelID); execErr != nil {
		err = fmt.Errorf("update channel models: %w", execErr)
		panic(err)
	}

	abilitySeeds, loadErr := loadTargetAbilities(tx, targetChannelID)
	if loadErr != nil {
		err = fmt.Errorf("load ability seeds: %w", loadErr)
		panic(err)
	}
	if len(abilitySeeds) == 0 {
		err = fmt.Errorf("channel %d has no ability groups to clone", targetChannelID)
		panic(err)
	}

	for _, seed := range abilitySeeds {
		if _, execErr := tx.Exec(
			`
INSERT INTO abilities (`+"`group`"+`, model, channel_id, enabled, priority, weight, tag)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  enabled = VALUES(enabled),
  priority = VALUES(priority),
  weight = VALUES(weight),
  tag = VALUES(tag)
			`,
			seed.Group,
			targetImageModelID,
			targetChannelID,
			seed.Enabled,
			seed.Priority,
			seed.Weight,
			seed.Tag,
		); execErr != nil {
			err = fmt.Errorf("upsert ability for group %s: %w", seed.Group, execErr)
			panic(err)
		}
	}

	modelRatios, ratioErr := loadModelRatios(tx)
	if ratioErr != nil {
		err = ratioErr
		panic(err)
	}
	modelRatios[targetImageModelID] = targetModelRatio

	ratioPayload, marshalErr := json.Marshal(modelRatios)
	if marshalErr != nil {
		err = fmt.Errorf("marshal ModelRatio JSON: %w", marshalErr)
		panic(err)
	}

	if _, execErr := tx.Exec(
		`
INSERT INTO options (`+"`key`"+`, value)
VALUES ('ModelRatio', ?)
ON DUPLICATE KEY UPDATE
  value = VALUES(value)
		`,
		string(ratioPayload),
	); execErr != nil {
		err = fmt.Errorf("upsert ModelRatio option: %w", execErr)
		panic(err)
	}

	if commitErr := tx.Commit(); commitErr != nil {
		err = fmt.Errorf("commit setup transaction: %w", commitErr)
		panic(err)
	}

	fmt.Printf(
		"Applied YiX image gateway setup: channel=%d model=%s groups=%d ratio=%.2f\n",
		targetChannelID,
		targetImageModelID,
		len(abilitySeeds),
		targetModelRatio,
	)
}
