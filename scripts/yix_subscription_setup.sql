-- YiX subscription bootstrap for new-api MySQL.
-- Creates four dedicated subscription groups, copies default abilities,
-- appends the groups to default channels, and upserts sixteen CNY plans.

START TRANSACTION;

SET @now_unix := UNIX_TIMESTAMP();

INSERT INTO options (`key`, `value`)
VALUES (
  'GroupRatio',
  '{"yix_basic_subscription":1,"yix_pro_subscription":1,"yix_ultimate_subscription":1,"yix_max_subscription":1}'
)
ON DUPLICATE KEY UPDATE
  `value` = CAST(
    JSON_SET(
      CASE
        WHEN JSON_VALID(`value`) THEN JSON_EXTRACT(`value`, '$')
        ELSE JSON_OBJECT()
      END,
      '$.yix_basic_subscription', 1,
      '$.yix_pro_subscription', 1,
      '$.yix_ultimate_subscription', 1,
      '$.yix_max_subscription', 1
    ) AS CHAR CHARACTER SET utf8mb4
  );

INSERT INTO options (`key`, `value`)
VALUES (
  'TopupGroupRatio',
  '{"yix_basic_subscription":1,"yix_pro_subscription":0.9090909091,"yix_ultimate_subscription":0.8333333333,"yix_max_subscription":0.7692307692}'
)
ON DUPLICATE KEY UPDATE
  `value` = CAST(
    JSON_SET(
      CASE
        WHEN JSON_VALID(`value`) THEN JSON_EXTRACT(`value`, '$')
        ELSE JSON_OBJECT()
      END,
      '$.yix_basic_subscription', 1,
      '$.yix_pro_subscription', 0.9090909091,
      '$.yix_ultimate_subscription', 0.8333333333,
      '$.yix_max_subscription', 0.7692307692
    ) AS CHAR CHARACTER SET utf8mb4
  );

ALTER TABLE channels
MODIFY COLUMN `group` varchar(255) DEFAULT 'default';

ALTER TABLE subscription_plans
MODIFY COLUMN `price_amount` decimal(14,2) NOT NULL DEFAULT 0;

UPDATE channels
SET `group` = TRIM(
  BOTH ','
  FROM CONCAT(
    REPLACE(IFNULL(`group`, ''), ' ', ''),
    IF(
      FIND_IN_SET('yix_basic_subscription', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0,
      '',
      ',yix_basic_subscription'
    ),
    IF(
      FIND_IN_SET('yix_pro_subscription', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0,
      '',
      ',yix_pro_subscription'
    ),
    IF(
      FIND_IN_SET('yix_ultimate_subscription', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0,
      '',
      ',yix_ultimate_subscription'
    ),
    IF(
      FIND_IN_SET('yix_max_subscription', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0,
      '',
      ',yix_max_subscription'
    )
  )
)
WHERE FIND_IN_SET('default', REPLACE(IFNULL(`group`, ''), ' ', '')) > 0;

INSERT INTO abilities (`group`, `model`, `channel_id`, `enabled`, `priority`, `weight`, `tag`)
SELECT target_groups.group_name, a.model, a.channel_id, a.enabled, a.priority, a.weight, a.tag
FROM abilities AS a
JOIN (
  SELECT 'yix_basic_subscription' AS group_name
  UNION ALL
  SELECT 'yix_pro_subscription'
  UNION ALL
  SELECT 'yix_ultimate_subscription'
  UNION ALL
  SELECT 'yix_max_subscription'
) AS target_groups
WHERE a.`group` = 'default'
ON DUPLICATE KEY UPDATE
  `enabled` = VALUES(`enabled`),
  `priority` = VALUES(`priority`),
  `weight` = VALUES(`weight`),
  `tag` = VALUES(`tag`);

INSERT INTO subscription_plans (
  `id`,
  `title`,
  `subtitle`,
  `price_amount`,
  `currency`,
  `duration_unit`,
  `duration_value`,
  `custom_seconds`,
  `enabled`,
  `sort_order`,
  `allow_balance_pay`,
  `stripe_price_id`,
  `creem_product_id`,
  `waffo_pancake_product_id`,
  `max_purchase_per_user`,
  `upgrade_group`,
  `total_amount`,
  `quota_reset_period`,
  `quota_reset_custom_seconds`,
  `created_at`,
  `updated_at`
)
VALUES
  (8101, 'Basic', 'YiX monthly plan: grants 1500 wallet credits; recharge bonus 0%', 90.00, 'CNY', 'month', 1, 0, 1, 8101, 0, '', '', '', 0, 'yix_basic_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8102, 'Basic', 'YiX yearly plan: grants 18000 wallet credits; recharge bonus 0%', 1071.00, 'CNY', 'year', 1, 0, 1, 8102, 0, '', '', '', 0, 'yix_basic_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8211, 'Pro 3.5K', 'YiX monthly plan: grants 3500 wallet credits; recharge bonus 10%', 184.00, 'CNY', 'month', 1, 0, 1, 8211, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8212, 'Pro 3.5K', 'YiX yearly plan: grants 42000 wallet credits; recharge bonus 10%', 2205.00, 'CNY', 'year', 1, 0, 1, 8212, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8201, 'Pro 6K', 'YiX monthly plan: grants 6000 wallet credits; recharge bonus 10%', 315.00, 'CNY', 'month', 1, 0, 1, 8201, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8202, 'Pro 6K', 'YiX yearly plan: grants 72000 wallet credits; recharge bonus 10%', 3780.00, 'CNY', 'year', 1, 0, 1, 8202, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8213, 'Pro 9.5K', 'YiX monthly plan: grants 9500 wallet credits; recharge bonus 10%', 499.00, 'CNY', 'month', 1, 0, 1, 8213, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8214, 'Pro 9.5K', 'YiX yearly plan: grants 114000 wallet credits; recharge bonus 10%', 5985.00, 'CNY', 'year', 1, 0, 1, 8214, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8215, 'Pro 11.5K', 'YiX monthly plan: grants 11500 wallet credits; recharge bonus 10%', 604.00, 'CNY', 'month', 1, 0, 1, 8215, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8216, 'Pro 11.5K', 'YiX yearly plan: grants 138000 wallet credits; recharge bonus 10%', 7245.00, 'CNY', 'year', 1, 0, 1, 8216, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8217, 'Pro 20K', 'YiX monthly plan: grants 20000 wallet credits; recharge bonus 10%', 1050.00, 'CNY', 'month', 1, 0, 1, 8217, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8218, 'Pro 20K', 'YiX yearly plan: grants 240000 wallet credits; recharge bonus 10%', 12600.00, 'CNY', 'year', 1, 0, 1, 8218, 0, '', '', '', 0, 'yix_pro_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8301, 'Ultimate', 'YiX monthly plan: grants 36000 wallet credits; recharge bonus 20%', 1512.00, 'CNY', 'month', 1, 0, 1, 8301, 0, '', '', '', 0, 'yix_ultimate_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8302, 'Ultimate', 'YiX yearly plan: grants 432000 wallet credits; recharge bonus 20%', 18144.00, 'CNY', 'year', 1, 0, 1, 8302, 0, '', '', '', 0, 'yix_ultimate_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8401, 'Max', 'YiX monthly plan: grants 72000 wallet credits; recharge bonus 30%', 3024.00, 'CNY', 'month', 1, 0, 1, 8401, 0, '', '', '', 0, 'yix_max_subscription', 1, 'never', 0, @now_unix, @now_unix),
  (8402, 'Max', 'YiX yearly plan: grants 864000 wallet credits; recharge bonus 30%', 36288.00, 'CNY', 'year', 1, 0, 1, 8402, 0, '', '', '', 0, 'yix_max_subscription', 1, 'never', 0, @now_unix, @now_unix)
ON DUPLICATE KEY UPDATE
  `title` = VALUES(`title`),
  `subtitle` = VALUES(`subtitle`),
  `price_amount` = VALUES(`price_amount`),
  `currency` = VALUES(`currency`),
  `duration_unit` = VALUES(`duration_unit`),
  `duration_value` = VALUES(`duration_value`),
  `custom_seconds` = VALUES(`custom_seconds`),
  `enabled` = VALUES(`enabled`),
  `sort_order` = VALUES(`sort_order`),
  `allow_balance_pay` = VALUES(`allow_balance_pay`),
  `stripe_price_id` = VALUES(`stripe_price_id`),
  `creem_product_id` = VALUES(`creem_product_id`),
  `waffo_pancake_product_id` = VALUES(`waffo_pancake_product_id`),
  `max_purchase_per_user` = VALUES(`max_purchase_per_user`),
  `upgrade_group` = VALUES(`upgrade_group`),
  `total_amount` = VALUES(`total_amount`),
  `quota_reset_period` = VALUES(`quota_reset_period`),
  `quota_reset_custom_seconds` = VALUES(`quota_reset_custom_seconds`),
  `updated_at` = VALUES(`updated_at`);

COMMIT;
