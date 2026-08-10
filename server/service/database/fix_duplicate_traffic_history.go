package database

import (
	"fmt"
	"oneclickvirt/global"
	"time"

	"go.uber.org/zap"
)

// FixDuplicateTrafficHistory 确认 instance_traffic_histories 表中的重复数据
// 这个函数用于清理老数据库中可能存在的重复记录
// 保留 ID 最小的记录，删除其他重复项
func (ds *DatabaseService) FixDuplicateTrafficHistory() error {
	db := ds.getDB()
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}

	// 防御性检查：表不存在时直接跳过（全新数据库无需确认）
	if !db.Migrator().HasTable("instance_traffic_histories") {
		global.APP_LOG.Info("instance_traffic_histories 表不存在，跳过重复数据检查（全新数据库）")
		return nil
	}

	global.APP_LOG.Info("开始检查并确认 instance_traffic_histories 表中的重复数据...")

	// 检查是否存在重复数据
	var duplicateCount int64
	checkSQL := `
		SELECT COUNT(*) as count FROM (
			SELECT instance_id, year, month, day, hour, COUNT(*) as cnt
			FROM instance_traffic_histories
			GROUP BY instance_id, year, month, day, hour
			HAVING cnt > 1
		) as duplicates
	`
	err := db.Raw(checkSQL).Scan(&duplicateCount).Error
	if err != nil {
		return fmt.Errorf("检查重复数据失败: %w", err)
	}

	if duplicateCount == 0 {
		global.APP_LOG.Info("未发现重复数据，无需确认")
		return nil
	}

	global.APP_LOG.Warn("发现重复数据组", zap.Int64("count", duplicateCount))

	// 删除重复数据，保留ID最小的记录
	// 使用临时表方法，兼容性更好
	deleteSQL := `
		DELETE t1 FROM instance_traffic_histories t1
		INNER JOIN (
			SELECT instance_id, year, month, day, hour, MIN(id) as min_id
			FROM instance_traffic_histories
			GROUP BY instance_id, year, month, day, hour
			HAVING COUNT(*) > 1
		) t2 
		ON t1.instance_id = t2.instance_id 
		AND t1.year = t2.year 
		AND t1.month = t2.month 
		AND t1.day = t2.day 
		AND t1.hour = t2.hour
		WHERE t1.id > t2.min_id
	`

	result := db.Exec(deleteSQL)
	if result.Error != nil {
		return fmt.Errorf("删除重复数据失败: %w", result.Error)
	}

	global.APP_LOG.Info("重复数据清理完成",
		zap.Int64("deleted_rows", result.RowsAffected))

	return nil
}

// FixAllDuplicateData 确认所有可能存在重复数据的表
func (ds *DatabaseService) FixAllDuplicateData() error {
	// 修复流量历史表重复数据
	if err := ds.FixDuplicateTrafficHistory(); err != nil {
		return err
	}
	// 迁移 ports 表唯一索引：将 (provider_id, host_port) 升级为 (provider_id, host_port, deleted_at)
	// 解决 GORM 软删除记录占用唯一索引槽位的问题
	if err := ds.MigratePortsIndex(); err != nil {
		return err
	}
	// 迁移 users 表 OAuth2 列名：将 GORM 默认生成的 o_auth2_* 重命名为 oauth2_*
	// 解决显式 column tag 与 GORM 默认命名策略不一致导致的"Unknown column"错误
	if err := ds.FixOAuth2ColumnNames(); err != nil {
		return err
	}
	return nil
}

// MigratePortsIndex 将 ports 表的唯一索引从双列 (provider_id, host_port)
// 迁移到三列 (provider_id, host_port, deleted_at)，使 GORM 软删除记录不再占用唯一索引槽位。
// 幂等操作：若新索引已存在或旧索引不存在，均安全跳过。
func (ds *DatabaseService) MigratePortsIndex() error {
	db := ds.getDB()
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	// 防御性检查：表不存在时跳过
	if !db.Migrator().HasTable("ports") {
		global.APP_LOG.Info("ports 表不存在，跳过索引迁移（全新数据库）")
		return nil
	}

	// 检查 idx_provider_host_port 索引的列数
	var columnCount int
	checkSQL := `
		SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE()
		  AND table_name = 'ports'
		  AND index_name = 'idx_provider_host_port'
	`
	row := sqlDB.QueryRow(checkSQL)
	if err := row.Scan(&columnCount); err != nil {
		return fmt.Errorf("检查 ports 索引失败: %w", err)
	}

	// 如果索引已经是 3 列（包含 deleted_at），说明已迁移，跳过
	if columnCount == 3 {
		global.APP_LOG.Info("ports 表索引已包含 deleted_at，无需迁移")
		return nil
	}

	if columnCount == 2 {
		global.APP_LOG.Info("发现旧的 ports 双列唯一索引，开始迁移到三列索引（含 deleted_at）")

		// 步骤1：硬删除所有软删除记录（它们已被标记删除，不应继续占用端口槽位）
		cleanResult, err := sqlDB.Exec("DELETE FROM ports WHERE deleted_at IS NOT NULL")
		if err != nil {
			global.APP_LOG.Warn("清理 ports 软删除记录失败", zap.Error(err))
		} else if rows, _ := cleanResult.RowsAffected(); rows > 0 {
			global.APP_LOG.Info("已清理 ports 软删除记录",
				zap.Int64("count", rows))
		}

		// 步骤2：删除旧的 2 列唯一索引
		if _, err := sqlDB.Exec("ALTER TABLE ports DROP INDEX `idx_provider_host_port`"); err != nil {
			return fmt.Errorf("删除旧索引 idx_provider_host_port 失败: %w", err)
		}
		global.APP_LOG.Info("旧索引 idx_provider_host_port 已删除")

		// 步骤3：创建新的 3 列唯一索引 (provider_id, host_port, deleted_at)
		createSQL := "ALTER TABLE ports ADD UNIQUE INDEX `idx_provider_host_port` (`provider_id`, `host_port`, `deleted_at`)"
		if _, err := sqlDB.Exec(createSQL); err != nil {
			return fmt.Errorf("创建新索引 idx_provider_host_port 失败: %w", err)
		}
		global.APP_LOG.Info("新索引 idx_provider_host_port (provider_id, host_port, deleted_at) 已创建")

		return nil
	}

	// 索引不存在（全新安装），AutoMigrate 会根据 struct tag 自动创建，无需处理
	global.APP_LOG.Info("ports 表尚未创建 idx_provider_host_port 索引，将由 AutoMigrate 创建")
	return nil
}

// FixSystemConfigDuplicates 清理 system_configs 的重复行，并确保唯一索引
// idx_system_configs_cat_key 落在 (category, key) 上（不含 deleted_at）。
//
// 历史问题：旧唯一索引为 (key, category, deleted_at)。由于 MySQL/MariaDB 将 NULL 视为互不相等，
// 所有 live 行的 deleted_at 均为 NULL，导致该唯一索引无法阻止 (category, key) 的重复插入。
// 于是每次重启的 syncYAMLConfigToDatabase / mergeYAMLDefaultsIntoDatabase / UpdateConfig
// 通过 ON DUPLICATE KEY UPDATE / INSERT IGNORE 追加新行（底层唯一索引永不冲突），
// 行数无限膨胀，最终 configCache 被空值覆盖（表现为「已配置 SMTP 但无法发邮件」）。
//
// 修复：
//  1. 去重：每个 (category, key) 仅保留最优一行（优先非空 value，其次 updated_at 最新，再次 id 最大）。
//  2. 重建唯一索引为 (category, key)，使 upsert 真正去重。
//
// 幂等：可重复安全执行；在数据库初始化阶段（FixSchemaColumns）调用，早于 ConfigManager 加载。
func (ds *DatabaseService) FixSystemConfigDuplicates() error {
	db := ds.getDB()
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}
	if !db.Migrator().HasTable("system_configs") {
		global.APP_LOG.Info("system_configs 表不存在，跳过重复数据修复（全新数据库）")
		return nil
	}

	// 1) 去重：在 Go 中计算保留行，避免依赖临时表与窗口函数（兼容连接池与低版本 MySQL）。
	type scRow struct {
		ID        uint
		Category  string
		Key       string
		Value     string
		UpdatedAt time.Time
	}
	var rows []scRow
	if err := db.Raw("SELECT id, category, `key`, value, updated_at FROM system_configs WHERE deleted_at IS NULL").Scan(&rows).Error; err != nil {
		return fmt.Errorf("读取 system_configs 失败: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	groups := make(map[string]*scRow)
	for i := range rows {
		r := &rows[i]
		gk := r.Category + "\x00" + r.Key
		g, ok := groups[gk]
		if !ok {
			groups[gk] = r
			continue
		}
		// 选择更优行：优先非空 value；其次 updated_at 更新；再次 id 更大
		better := false
		if (r.Value != "") && (g.Value == "") {
			better = true
		} else if (r.Value != "") == (g.Value != "") {
			if r.UpdatedAt.After(g.UpdatedAt) || (r.UpdatedAt.Equal(g.UpdatedAt) && r.ID > g.ID) {
				better = true
			}
		}
		if better {
			groups[gk] = r
		}
	}

	keep := make(map[uint]bool, len(groups))
	for _, g := range groups {
		keep[g.ID] = true
	}
	var delIDs []uint
	for i := range rows {
		if !keep[rows[i].ID] {
			delIDs = append(delIDs, rows[i].ID)
		}
	}
	if len(delIDs) > 0 {
		for s := 0; s < len(delIDs); s += 1000 {
			batch := delIDs[s:]
			if len(batch) > 1000 {
				batch = batch[:1000]
			}
			if err := db.Exec("DELETE FROM system_configs WHERE id IN (?)", batch).Error; err != nil {
				return fmt.Errorf("清理 system_configs 重复行失败: %w", err)
			}
		}
		global.APP_LOG.Warn("已清理 system_configs 重复配置行", zap.Int("count", len(delIDs)))
	}

	// 2) 确保唯一索引为 (category, key)（不含 deleted_at，避免 NULL 互异导致无法去重）。
	var colCount int
	row := sqlDB.QueryRow(`
		SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE()
		  AND table_name = 'system_configs'
		  AND index_name = 'idx_system_configs_cat_key'
	`)
	if err := row.Scan(&colCount); err != nil {
		return fmt.Errorf("检查 system_configs 索引失败: %w", err)
	}

	if colCount == 0 {
		if _, err := sqlDB.Exec("ALTER TABLE system_configs ADD UNIQUE INDEX `idx_system_configs_cat_key` (`category`, `key`)"); err != nil {
			return fmt.Errorf("创建 system_configs 唯一索引失败: %w", err)
		}
		global.APP_LOG.Info("system_configs 唯一索引已创建 (category, key)")
		return nil
	}

	// 读取现有索引列顺序
	cols, err := sqlDB.Query(`
		SELECT column_name FROM information_schema.STATISTICS
		WHERE table_schema = DATABASE()
		  AND table_name = 'system_configs'
		  AND index_name = 'idx_system_configs_cat_key'
		ORDER BY seq_in_index
	`)
	if err != nil {
		return fmt.Errorf("读取 system_configs 索引列失败: %w", err)
	}
	var colNames []string
	for cols.Next() {
		var c string
		if err := cols.Scan(&c); err != nil {
			cols.Close()
			return fmt.Errorf("读取 system_configs 索引列失败: %w", err)
		}
		colNames = append(colNames, c)
	}
	cols.Close()

	// 期望列顺序: [category, key]
	needFix := len(colNames) != 2 || colNames[0] != "category" || colNames[1] != "key"
	if !needFix {
		global.APP_LOG.Info("system_configs 唯一索引已正确 (category, key)，无需重建")
		return nil
	}

	if _, err := sqlDB.Exec("ALTER TABLE system_configs DROP INDEX `idx_system_configs_cat_key`"); err != nil {
		return fmt.Errorf("删除旧 system_configs 索引失败: %w", err)
	}
	if _, err := sqlDB.Exec("ALTER TABLE system_configs ADD UNIQUE INDEX `idx_system_configs_cat_key` (`category`, `key`)"); err != nil {
		return fmt.Errorf("重建 system_configs 唯一索引失败: %w", err)
	}
	global.APP_LOG.Info("system_configs 唯一索引已重建为 (category, key)")
	return nil
}
