package main

import (
	"database/sql"
	"fmt"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const (
	defaultNewAPIDSN = "root:toor0109@tcp(localhost:3306)/new_api?charset=utf8mb4&parseTime=true&loc=Local"
	defaultYiXDSN    = "root:toor0109@tcp(localhost:3306)/yix?charset=utf8mb4&parseTime=true&loc=Local"
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

func fetchCount(db *sql.DB, query string, args ...any) int {
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		panic(err)
	}
	return count
}

func fetchString(db *sql.DB, query string, args ...any) string {
	var value string
	if err := db.QueryRow(query, args...).Scan(&value); err != nil {
		panic(err)
	}
	return value
}

func requirePrice(db *sql.DB, planId int, expected string) {
	actual := fetchString(
		db,
		"SELECT CAST(`price_amount` AS CHAR) FROM subscription_plans WHERE id = ?",
		planId,
	)
	if actual != expected {
		panic(fmt.Errorf("unexpected plan price: plan_id=%d expected=%s actual=%s", planId, expected, actual))
	}
}

func requireContains(label, value string, required ...string) {
	for _, item := range required {
		if !strings.Contains(value, item) {
			panic(fmt.Errorf("%s missing %q", label, item))
		}
	}
}

func main() {
	newAPIDB := mustOpen("new_api", envOrDefault("NEW_API_VERIFY_DSN", defaultNewAPIDSN))
	defer newAPIDB.Close()

	yixDB := mustOpen("yix", envOrDefault("YIX_VERIFY_DSN", defaultYiXDSN))
	defer yixDB.Close()

	planCount := fetchCount(
		newAPIDB,
		"SELECT COUNT(*) FROM subscription_plans WHERE id IN (8101,8102,8211,8212,8201,8202,8213,8214,8215,8216,8217,8218,8301,8302,8401,8402)",
	)
	if planCount != 16 {
		panic(fmt.Errorf("expected 16 subscription plans, got %d", planCount))
	}

	requirePrice(newAPIDB, 8211, "184.00")
	requirePrice(newAPIDB, 8212, "2205.00")
	requirePrice(newAPIDB, 8201, "315.00")
	requirePrice(newAPIDB, 8202, "3780.00")
	requirePrice(newAPIDB, 8213, "499.00")
	requirePrice(newAPIDB, 8214, "5985.00")
	requirePrice(newAPIDB, 8215, "604.00")
	requirePrice(newAPIDB, 8216, "7245.00")
	requirePrice(newAPIDB, 8217, "1050.00")
	requirePrice(newAPIDB, 8218, "12600.00")

	abilityCount := fetchCount(
		newAPIDB,
		"SELECT COUNT(*) FROM abilities WHERE `group` IN ('yix_basic_subscription','yix_pro_subscription','yix_ultimate_subscription','yix_max_subscription')",
	)
	if abilityCount == 0 {
		panic("expected cloned abilities for YiX subscription groups")
	}

	channelCount := fetchCount(
		newAPIDB,
		"SELECT COUNT(*) FROM channels WHERE FIND_IN_SET('yix_basic_subscription', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0",
	)
	if channelCount == 0 {
		panic("expected at least one channel bound to YiX subscription groups")
	}

	groupRatioValue := fetchString(
		newAPIDB,
		"SELECT `value` FROM options WHERE `key` = 'GroupRatio'",
	)
	requireContains(
		"GroupRatio",
		groupRatioValue,
		"yix_basic_subscription",
		"yix_pro_subscription",
		"yix_ultimate_subscription",
		"yix_max_subscription",
	)

	topupRatioValue := fetchString(
		newAPIDB,
		"SELECT `value` FROM options WHERE `key` = 'TopupGroupRatio'",
	)
	requireContains(
		"TopupGroupRatio",
		topupRatioValue,
		"yix_basic_subscription",
		"yix_pro_subscription",
		"yix_ultimate_subscription",
		"yix_max_subscription",
	)

	planMapValue := fetchString(
		yixDB,
		"SELECT `value` FROM systemconfig WHERE `key` = 'NEW_API_SUBSCRIPTION_PLAN_MAP'",
	)
	requireContains(
		"NEW_API_SUBSCRIPTION_PLAN_MAP",
		planMapValue,
		"\"basic_monthly\":8101",
		"\"pro_monthly_3500\":8211",
		"\"pro_yearly_3500\":8212",
		"\"pro_monthly_9500\":8213",
		"\"pro_yearly_9500\":8214",
		"\"pro_monthly_11500\":8215",
		"\"pro_yearly_11500\":8216",
		"\"pro_monthly_20000\":8217",
		"\"pro_yearly_20000\":8218",
		"\"max_yearly\":8402",
	)

	creditsMapValue := fetchString(
		yixDB,
		"SELECT `value` FROM systemconfig WHERE `key` = 'YIX_SUBSCRIPTION_CREDITS_MAP'",
	)
	requireContains(
		"YIX_SUBSCRIPTION_CREDITS_MAP",
		creditsMapValue,
		"\"pro_monthly_3500\":3500",
		"\"pro_yearly_3500\":3500",
		"\"pro_monthly_20000\":20000",
		"\"pro_yearly_20000\":20000",
	)

	fmt.Printf("Verified YiX subscription setup: plans=%d, cloned_abilities=%d, bound_channels=%d\n", planCount, abilityCount, channelCount)
}
