package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	SubscriptionStatusActive    = "active"
	SubscriptionStatusScheduled = "scheduled"
	SubscriptionStatusExpired   = "expired"
	SubscriptionStatusCancelled = "cancelled"
)

const (
	SubscriptionScenarioFirstPurchase      = "first_purchase"
	SubscriptionScenarioSamePlanRenewal    = "same_plan_renewal"
	SubscriptionScenarioSameCycleUpgrade   = "same_cycle_upgrade"
	SubscriptionScenarioSameCycleDowngrade = "same_cycle_downgrade"
	SubscriptionScenarioCrossCycleSwitch   = "cross_cycle_switch"
)

var (
	ErrSubscriptionStateChanged        = errors.New("subscription state changed")
	ErrSubscriptionPendingChangeExists = errors.New("scheduled subscription already exists")
	ErrSubscriptionScenarioInvalid     = errors.New("invalid subscription scenario")
)

type SubscriptionOrderLifecycle struct {
	Scenario              string `json:"scenario"`
	CurrentSubscriptionId int    `json:"current_subscription_id,omitempty"`
	CurrentPlanId         int    `json:"current_plan_id,omitempty"`
	CurrentEndTime        int64  `json:"current_end_time,omitempty"`
	FallbackGroup         string `json:"fallback_group,omitempty"`
}

type SubscriptionPurchasePolicyInput struct {
	Scenario               string
	CurrentSubscriptionId  int
	ExpectedCurrentPlanId  int
	ExpectedCurrentEndTime int64
	OverrideMoney          float64
}

type PreparedSubscriptionPurchase struct {
	Money     float64
	Lifecycle SubscriptionOrderLifecycle
}

type CreateUserSubscriptionParams struct {
	UserId        int
	Plan          *SubscriptionPlan
	Source        string
	StartTime     int64
	EndTime       int64
	Status        string
	PrevUserGroup string
	ApplyGroup    bool
}

func NormalizeSubscriptionScenario(value string) string {
	switch strings.TrimSpace(value) {
	case "", SubscriptionScenarioFirstPurchase:
		return SubscriptionScenarioFirstPurchase
	case SubscriptionScenarioSamePlanRenewal:
		return SubscriptionScenarioSamePlanRenewal
	case SubscriptionScenarioSameCycleUpgrade:
		return SubscriptionScenarioSameCycleUpgrade
	case SubscriptionScenarioSameCycleDowngrade:
		return SubscriptionScenarioSameCycleDowngrade
	case SubscriptionScenarioCrossCycleSwitch:
		return SubscriptionScenarioCrossCycleSwitch
	default:
		return ""
	}
}

func normalizeMoneyAmount(value float64) float64 {
	return decimal.NewFromFloat(value).Round(2).InexactFloat64()
}

func subscriptionCycleSignature(plan *SubscriptionPlan) string {
	if plan == nil {
		return ""
	}
	switch plan.DurationUnit {
	case SubscriptionDurationCustom:
		return fmt.Sprintf("%s:%d", SubscriptionDurationCustom, plan.CustomSeconds)
	default:
		return fmt.Sprintf("%s:%d", strings.TrimSpace(plan.DurationUnit), plan.DurationValue)
	}
}

func isSameSubscriptionCycle(left *SubscriptionPlan, right *SubscriptionPlan) bool {
	return subscriptionCycleSignature(left) != "" && subscriptionCycleSignature(left) == subscriptionCycleSignature(right)
}

func hasScheduledUserSubscriptionTx(tx *gorm.DB, userId int, now int64) (bool, error) {
	if tx == nil {
		return false, errors.New("tx is nil")
	}
	var count int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, SubscriptionStatusScheduled, now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func hasActiveUserSubscriptionTx(tx *gorm.DB, userId int, now int64) (bool, error) {
	if tx == nil {
		return false, errors.New("tx is nil")
	}
	var count int64
	if err := tx.Model(&UserSubscription{}).
		Where("user_id = ? AND status = ? AND end_time > ?", userId, SubscriptionStatusActive, now).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func getActiveUserSubscriptionWithPlanTx(tx *gorm.DB, userId int, subscriptionId int, now int64) (*UserSubscription, *SubscriptionPlan, error) {
	if tx == nil {
		return nil, nil, errors.New("tx is nil")
	}
	if subscriptionId <= 0 {
		return nil, nil, ErrSubscriptionStateChanged
	}
	var sub UserSubscription
	query := tx.Set("gorm:query_option", "FOR UPDATE").
		Where("id = ? AND user_id = ? AND status = ? AND end_time > ?", subscriptionId, userId, SubscriptionStatusActive, now).
		First(&sub)
	if query.Error != nil {
		if errors.Is(query.Error, gorm.ErrRecordNotFound) {
			return nil, nil, ErrSubscriptionStateChanged
		}
		return nil, nil, query.Error
	}
	plan, err := getSubscriptionPlanByIdTx(tx, sub.PlanId)
	if err != nil {
		return nil, nil, err
	}
	return &sub, plan, nil
}

func PrepareSubscriptionPurchaseTx(tx *gorm.DB, userId int, plan *SubscriptionPlan, input SubscriptionPurchasePolicyInput) (*PreparedSubscriptionPurchase, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if plan == nil || plan.Id <= 0 {
		return nil, errors.New("invalid plan")
	}
	scenario := NormalizeSubscriptionScenario(input.Scenario)
	if scenario == "" {
		return nil, ErrSubscriptionScenarioInvalid
	}
	now := getDBTimestampWithDB(tx)
	hasScheduled, err := hasScheduledUserSubscriptionTx(tx, userId, now)
	if err != nil {
		return nil, err
	}
	if hasScheduled {
		return nil, ErrSubscriptionPendingChangeExists
	}

	money := normalizeMoneyAmount(plan.PriceAmount)
	if scenario == SubscriptionScenarioSameCycleUpgrade {
		money = normalizeMoneyAmount(input.OverrideMoney)
		if money < 0.01 {
			return nil, errors.New("升级补差金额过低")
		}
	}
	if money < 0.01 {
		return nil, errors.New("套餐金额过低")
	}

	if scenario == SubscriptionScenarioFirstPurchase {
		hasActive, err := hasActiveUserSubscriptionTx(tx, userId, now)
		if err != nil {
			return nil, err
		}
		if hasActive {
			return nil, ErrSubscriptionStateChanged
		}
		fallbackGroup, err := getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
		return &PreparedSubscriptionPurchase{
			Money: money,
			Lifecycle: SubscriptionOrderLifecycle{
				Scenario:      scenario,
				FallbackGroup: strings.TrimSpace(fallbackGroup),
			},
		}, nil
	}

	currentSub, currentPlan, err := getActiveUserSubscriptionWithPlanTx(tx, userId, input.CurrentSubscriptionId, now)
	if err != nil {
		return nil, err
	}
	if input.ExpectedCurrentPlanId > 0 && currentSub.PlanId != input.ExpectedCurrentPlanId {
		return nil, ErrSubscriptionStateChanged
	}
	if input.ExpectedCurrentEndTime > 0 && currentSub.EndTime != input.ExpectedCurrentEndTime {
		return nil, ErrSubscriptionStateChanged
	}

	switch scenario {
	case SubscriptionScenarioSamePlanRenewal:
		if currentSub.PlanId != plan.Id {
			return nil, ErrSubscriptionScenarioInvalid
		}
	case SubscriptionScenarioSameCycleUpgrade:
		if currentSub.PlanId == plan.Id || !isSameSubscriptionCycle(currentPlan, plan) {
			return nil, ErrSubscriptionScenarioInvalid
		}
	case SubscriptionScenarioSameCycleDowngrade:
		if currentSub.PlanId == plan.Id || !isSameSubscriptionCycle(currentPlan, plan) {
			return nil, ErrSubscriptionScenarioInvalid
		}
	case SubscriptionScenarioCrossCycleSwitch:
		if isSameSubscriptionCycle(currentPlan, plan) {
			return nil, ErrSubscriptionScenarioInvalid
		}
	default:
		return nil, ErrSubscriptionScenarioInvalid
	}

	fallbackGroup := strings.TrimSpace(currentSub.PrevUserGroup)
	if fallbackGroup == "" {
		fallbackGroup, err = getUserGroupByIdTx(tx, userId)
		if err != nil {
			return nil, err
		}
	}

	return &PreparedSubscriptionPurchase{
		Money: money,
		Lifecycle: SubscriptionOrderLifecycle{
			Scenario:              scenario,
			CurrentSubscriptionId: currentSub.Id,
			CurrentPlanId:         currentSub.PlanId,
			CurrentEndTime:        currentSub.EndTime,
			FallbackGroup:         strings.TrimSpace(fallbackGroup),
		},
	}, nil
}

func (o *SubscriptionOrder) SetLifecycle(lifecycle SubscriptionOrderLifecycle) error {
	if o == nil {
		return errors.New("subscription order is nil")
	}
	lifecycle.Scenario = NormalizeSubscriptionScenario(lifecycle.Scenario)
	if lifecycle.Scenario == "" {
		lifecycle.Scenario = SubscriptionScenarioFirstPurchase
	}
	raw, err := json.Marshal(lifecycle)
	if err != nil {
		return err
	}
	o.LifecycleScenario = lifecycle.Scenario
	o.LifecyclePayload = string(raw)
	return nil
}

func (o *SubscriptionOrder) GetLifecycle() SubscriptionOrderLifecycle {
	if o == nil {
		return SubscriptionOrderLifecycle{
			Scenario: SubscriptionScenarioFirstPurchase,
		}
	}
	lifecycle := SubscriptionOrderLifecycle{
		Scenario: NormalizeSubscriptionScenario(o.LifecycleScenario),
	}
	if strings.TrimSpace(o.LifecyclePayload) != "" {
		var parsed SubscriptionOrderLifecycle
		if err := json.Unmarshal([]byte(o.LifecyclePayload), &parsed); err == nil {
			if normalized := NormalizeSubscriptionScenario(parsed.Scenario); normalized != "" {
				parsed.Scenario = normalized
			} else if lifecycle.Scenario != "" {
				parsed.Scenario = lifecycle.Scenario
			} else {
				parsed.Scenario = SubscriptionScenarioFirstPurchase
			}
			return parsed
		}
	}
	if lifecycle.Scenario == "" {
		lifecycle.Scenario = SubscriptionScenarioFirstPurchase
	}
	return lifecycle
}

func CreateUserSubscriptionWithSpecTx(tx *gorm.DB, params CreateUserSubscriptionParams) (*UserSubscription, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if params.Plan == nil || params.Plan.Id == 0 {
		return nil, errors.New("invalid plan")
	}
	if params.UserId <= 0 {
		return nil, errors.New("invalid user id")
	}
	if params.StartTime <= 0 || params.EndTime <= params.StartTime {
		return nil, errors.New("invalid subscription time window")
	}
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = SubscriptionStatusActive
	}
	if status != SubscriptionStatusActive && status != SubscriptionStatusScheduled {
		return nil, errors.New("invalid subscription status")
	}
	if params.Plan.MaxPurchasePerUser > 0 {
		var count int64
		if err := tx.Model(&UserSubscription{}).
			Where("user_id = ? AND plan_id = ?", params.UserId, params.Plan.Id).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count >= int64(params.Plan.MaxPurchasePerUser) {
			return nil, errors.New("已达到该套餐购买上限")
		}
	}

	startTime := time.Unix(params.StartTime, 0)
	nextReset := calcNextResetTime(startTime, params.Plan, params.EndTime)
	lastReset := int64(0)
	if nextReset > 0 {
		lastReset = params.StartTime
	}

	upgradeGroup := strings.TrimSpace(params.Plan.UpgradeGroup)
	prevGroup := strings.TrimSpace(params.PrevUserGroup)
	if prevGroup == "" {
		currentGroup, err := getUserGroupByIdTx(tx, params.UserId)
		if err != nil {
			return nil, err
		}
		prevGroup = strings.TrimSpace(currentGroup)
	}
	if params.ApplyGroup && upgradeGroup != "" {
		currentGroup, err := getUserGroupByIdTx(tx, params.UserId)
		if err != nil {
			return nil, err
		}
		if currentGroup != upgradeGroup {
			if err := tx.Model(&User{}).Where("id = ?", params.UserId).
				Update("group", upgradeGroup).Error; err != nil {
				return nil, err
			}
		}
	}

	sub := &UserSubscription{
		UserId:        params.UserId,
		PlanId:        params.Plan.Id,
		AmountTotal:   params.Plan.TotalAmount,
		AmountUsed:    0,
		StartTime:     params.StartTime,
		EndTime:       params.EndTime,
		Status:        status,
		Source:        strings.TrimSpace(params.Source),
		LastResetTime: lastReset,
		NextResetTime: nextReset,
		UpgradeGroup:  upgradeGroup,
		PrevUserGroup: prevGroup,
	}
	if sub.Source == "" {
		sub.Source = "order"
	}
	if err := tx.Create(sub).Error; err != nil {
		return nil, err
	}
	return sub, nil
}

func applySubscriptionOrderLifecycleTx(tx *gorm.DB, order *SubscriptionOrder, plan *SubscriptionPlan) (string, error) {
	if tx == nil || order == nil || plan == nil {
		return "", errors.New("invalid subscription lifecycle args")
	}
	now := getDBTimestampWithDB(tx)
	lifecycle := order.GetLifecycle()
	switch lifecycle.Scenario {
	case SubscriptionScenarioSamePlanRenewal, SubscriptionScenarioSameCycleDowngrade, SubscriptionScenarioCrossCycleSwitch:
		if lifecycle.CurrentEndTime <= 0 {
			return "", errors.New("subscription lifecycle missing current_end_time")
		}
		endTime, err := calcPlanEndTime(time.Unix(lifecycle.CurrentEndTime, 0), plan)
		if err != nil {
			return "", err
		}
		status := SubscriptionStatusScheduled
		applyGroup := false
		if lifecycle.CurrentEndTime <= now {
			if endTime <= now {
				return "", errors.New("subscription window has already ended")
			}
			status = SubscriptionStatusActive
			applyGroup = true
		}
		if _, err := CreateUserSubscriptionWithSpecTx(tx, CreateUserSubscriptionParams{
			UserId:        order.UserId,
			Plan:          plan,
			Source:        "order",
			StartTime:     lifecycle.CurrentEndTime,
			EndTime:       endTime,
			Status:        status,
			PrevUserGroup: lifecycle.FallbackGroup,
			ApplyGroup:    applyGroup,
		}); err != nil {
			return "", err
		}
		if applyGroup {
			return strings.TrimSpace(plan.UpgradeGroup), nil
		}
		return "", nil
	case SubscriptionScenarioSameCycleUpgrade:
		if lifecycle.CurrentEndTime <= now {
			return "", errors.New("当前订阅周期已结束，请重新发起购买")
		}
		if lifecycle.CurrentSubscriptionId > 0 {
			if err := tx.Model(&UserSubscription{}).
				Where("id = ? AND user_id = ? AND status = ? AND end_time > ?", lifecycle.CurrentSubscriptionId, order.UserId, SubscriptionStatusActive, now).
				Updates(map[string]interface{}{
					"status":     SubscriptionStatusCancelled,
					"end_time":   now,
					"updated_at": common.GetTimestamp(),
				}).Error; err != nil {
				return "", err
			}
		}
		if _, err := CreateUserSubscriptionWithSpecTx(tx, CreateUserSubscriptionParams{
			UserId:        order.UserId,
			Plan:          plan,
			Source:        "order",
			StartTime:     now,
			EndTime:       lifecycle.CurrentEndTime,
			Status:        SubscriptionStatusActive,
			PrevUserGroup: lifecycle.FallbackGroup,
			ApplyGroup:    true,
		}); err != nil {
			return "", err
		}
		return strings.TrimSpace(plan.UpgradeGroup), nil
	case SubscriptionScenarioFirstPurchase:
		fallthrough
	default:
		sub, err := CreateUserSubscriptionFromPlanTx(tx, order.UserId, plan, "order")
		if err != nil {
			return "", err
		}
		if sub != nil {
			return strings.TrimSpace(sub.UpgradeGroup), nil
		}
		return strings.TrimSpace(plan.UpgradeGroup), nil
	}
}

func reconcileSubscriptionUserGroupTx(tx *gorm.DB, userId int, now int64) (string, error) {
	if tx == nil {
		return "", errors.New("tx is nil")
	}
	if userId <= 0 {
		return "", errors.New("invalid user id")
	}
	currentGroup, err := getUserGroupByIdTx(tx, userId)
	if err != nil {
		return "", err
	}
	var activeSub UserSubscription
	activeQuery := tx.Where("user_id = ? AND status = ? AND end_time > ? AND upgrade_group <> ''",
		userId, SubscriptionStatusActive, now).
		Order("start_time desc, end_time desc, id desc").
		Limit(1).
		Find(&activeSub)
	if activeQuery.Error != nil {
		return "", activeQuery.Error
	}
	if activeQuery.RowsAffected > 0 {
		targetGroup := strings.TrimSpace(activeSub.UpgradeGroup)
		if targetGroup == "" {
			return "", nil
		}
		if currentGroup != targetGroup {
			if err := tx.Model(&User{}).Where("id = ?", userId).
				Update("group", targetGroup).Error; err != nil {
				return "", err
			}
		}
		return targetGroup, nil
	}

	var lastSub UserSubscription
	lastQuery := tx.Where("user_id = ? AND status IN ? AND upgrade_group <> ''",
		userId, []string{SubscriptionStatusExpired, SubscriptionStatusCancelled}).
		Order("end_time desc, updated_at desc, id desc").
		Limit(1).
		Find(&lastSub)
	if lastQuery.Error != nil {
		return "", lastQuery.Error
	}
	if lastQuery.RowsAffected == 0 {
		return "", nil
	}
	targetGroup := strings.TrimSpace(lastSub.PrevUserGroup)
	if targetGroup == "" {
		return "", nil
	}
	if currentGroup != targetGroup {
		if err := tx.Model(&User{}).Where("id = ?", userId).
			Update("group", targetGroup).Error; err != nil {
			return "", err
		}
	}
	return targetGroup, nil
}

func SyncDueUserSubscriptions(limit int) (int, int, error) {
	if limit <= 0 {
		limit = 200
	}
	now := GetDBTimestamp()
	var candidates []UserSubscription
	if err := DB.Select("user_id").
		Where("(status = ? AND end_time > 0 AND end_time <= ?) OR (status = ? AND start_time > 0 AND start_time <= ?)",
			SubscriptionStatusActive, now, SubscriptionStatusScheduled, now).
		Order("user_id asc").
		Limit(limit).
		Find(&candidates).Error; err != nil {
		return 0, 0, err
	}
	if len(candidates) == 0 {
		return 0, 0, nil
	}

	userIds := make([]int, 0, len(candidates))
	seen := make(map[int]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.UserId <= 0 {
			continue
		}
		if _, exists := seen[candidate.UserId]; exists {
			continue
		}
		seen[candidate.UserId] = struct{}{}
		userIds = append(userIds, candidate.UserId)
	}

	activatedCount := 0
	expiredCount := 0
	for _, userId := range userIds {
		cacheGroup := ""
		err := DB.Transaction(func(tx *gorm.DB) error {
			res := tx.Model(&UserSubscription{}).
				Where("user_id = ? AND status = ? AND end_time > 0 AND end_time <= ?", userId, SubscriptionStatusActive, now).
				Updates(map[string]interface{}{
					"status":     SubscriptionStatusExpired,
					"updated_at": common.GetTimestamp(),
				})
			if res.Error != nil {
				return res.Error
			}
			expiredCount += int(res.RowsAffected)

			var scheduledSubs []UserSubscription
			if err := tx.Set("gorm:query_option", "FOR UPDATE").
				Where("user_id = ? AND status = ? AND start_time > 0 AND start_time <= ?", userId, SubscriptionStatusScheduled, now).
				Order("start_time asc, id asc").
				Find(&scheduledSubs).Error; err != nil {
				return err
			}
			for _, scheduledSub := range scheduledSubs {
				nextStatus := SubscriptionStatusActive
				if scheduledSub.EndTime > 0 && scheduledSub.EndTime <= now {
					nextStatus = SubscriptionStatusExpired
				}
				if err := tx.Model(&UserSubscription{}).Where("id = ?", scheduledSub.Id).Updates(map[string]interface{}{
					"status":     nextStatus,
					"updated_at": common.GetTimestamp(),
				}).Error; err != nil {
					return err
				}
				if nextStatus == SubscriptionStatusActive {
					activatedCount++
				} else {
					expiredCount++
				}
			}
			targetGroup, err := reconcileSubscriptionUserGroupTx(tx, userId, now)
			if err != nil {
				return err
			}
			cacheGroup = targetGroup
			return nil
		})
		if err != nil {
			return activatedCount, expiredCount, err
		}
		if cacheGroup != "" {
			_ = UpdateUserGroupCache(userId, cacheGroup)
		}
	}

	return activatedCount, expiredCount, nil
}
