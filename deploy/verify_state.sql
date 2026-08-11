-- 每个 auth.email-* key 的行数
SELECT `key`, COUNT(*) AS cnt FROM system_configs WHERE `key` LIKE 'auth.email-%' GROUP BY `key` ORDER BY `key`;

-- 全表行数
SELECT COUNT(*) AS total_rows FROM system_configs;

-- 确认正确值存在（按 key+value 去重看实际值）
SELECT `key`, `value` FROM system_configs
WHERE `key` IN ('auth.enable-email','auth.email-smtp-host','auth.email-smtp-port','auth.email-username','auth.email-password')
GROUP BY `key`, `value`;

-- 实际唯一 (category,key) 组合数 vs 总行数，判断是否有重复
SELECT
  (SELECT COUNT(*) FROM system_configs WHERE deleted_at IS NULL) AS live_rows,
  (SELECT COUNT(DISTINCT CONCAT(category,'#',`key`)) FROM system_configs WHERE deleted_at IS NULL) AS distinct_pairs;

-- 显示当前索引（确认是否存在真正生效的唯一索引）
SHOW INDEX FROM system_configs;
