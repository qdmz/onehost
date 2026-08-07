package database

import (
	"fmt"

	"oneclickvirt/global"

	"go.uber.org/zap"
)

// FixSchemaColumns 执行幂等的数据库列结构修复迁移。
// 该函数在系统初始化时调用，确保已有的表结构与新版本代码期望的一致。
//
// 包含以下迁移：
//  1. site_configs.primary_color 列扩展为 VARCHAR(64)（仅当当前长度 < 64 时）
//  2. products 表添加 stock 列（INT DEFAULT -1）
//  3. products 表添加 max_per_user 列（INT DEFAULT 0）
//  4. 修复 users 表中 level=999 的无效用户等级，将其设为 1
//
// 该操作为幂等操作，安全可重复执行。
// 关键迁移（products 列添加）失败时返回错误，非关键迁移失败时仅记录警告。
func (ds *DatabaseService) FixSchemaColumns() error {
	db := ds.getDB()
	if db == nil {
		return fmt.Errorf("数据库连接不可用")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("获取底层数据库连接失败: %w", err)
	}

	// === 1. 扩展 site_configs.primary_color 列为 VARCHAR(64) ===
	// 旧版本中 primary_color 可能是 VARCHAR(20) 等较短长度，
	// 新版本需要存储更长的颜色值，因此扩展为 VARCHAR(64)。
	// 非关键迁移：失败时记录警告但不中断后续迁移。
	if db.Migrator().HasTable("site_configs") {
		columns, err := ds.getTableColumns(db, "site_configs")
		if err != nil {
			global.APP_LOG.Warn("获取 site_configs 表列信息失败，跳过 primary_color 列扩展", zap.Error(err))
		} else if columns["primary_color"] {
			// 查询当前列的 VARCHAR 长度
			var maxLength *int
			if err := db.Raw(
				"SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.COLUMNS "+
					"WHERE table_schema = DATABASE() AND table_name = 'site_configs' AND column_name = 'primary_color'",
			).Scan(&maxLength).Error; err != nil {
				global.APP_LOG.Warn("查询 primary_color 列长度失败，跳过扩展", zap.Error(err))
			} else if maxLength == nil || *maxLength < 64 {
				currentLen := 0
				if maxLength != nil {
					currentLen = *maxLength
				}
				global.APP_LOG.Info("正在扩展 site_configs.primary_color 列为 VARCHAR(64)",
					zap.Int("current_length", currentLen))
				if _, err := sqlDB.Exec(
					"ALTER TABLE `site_configs` MODIFY COLUMN `primary_color` VARCHAR(64) DEFAULT '#409EFF'",
				); err != nil {
					global.APP_LOG.Warn("扩展 primary_color 列失败", zap.Error(err))
				} else {
					global.APP_LOG.Info("site_configs.primary_color 列扩展完成")
				}
			} else {
				global.APP_LOG.Debug("primary_color 列长度已满足要求，跳过扩展",
					zap.Int("current_length", *maxLength))
			}
		} else {
			global.APP_LOG.Debug("site_configs.primary_color 列不存在，跳过扩展")
		}
	} else {
		global.APP_LOG.Debug("site_configs 表不存在，跳过 primary_color 列扩展（全新数据库）")
	}

	// === 2 & 3. 为 products 表添加缺失列 ===
	// stock 和 max_per_user 是新版本添加的列，旧数据库可能不存在。
	// 关键迁移：失败时返回错误，因为应用代码依赖这些列。
	if db.Migrator().HasTable("products") {
		columns, err := ds.getTableColumns(db, "products")
		if err != nil {
			return fmt.Errorf("获取 products 表列信息失败: %w", err)
		}

		// 添加 stock 列（库存，-1 表示不限）
		if !columns["stock"] {
			global.APP_LOG.Info("正在为 products 表添加 stock 列")
			if _, err := sqlDB.Exec("ALTER TABLE `products` ADD COLUMN `stock` INT DEFAULT -1"); err != nil {
				return fmt.Errorf("添加 products.stock 列失败: %w", err)
			}
			global.APP_LOG.Info("products.stock 列添加完成")
		} else {
			global.APP_LOG.Debug("products.stock 列已存在，跳过")
		}

		// 添加 max_per_user 列（每用户最大购买数，0 表示不限）
		if !columns["max_per_user"] {
			global.APP_LOG.Info("正在为 products 表添加 max_per_user 列")
			if _, err := sqlDB.Exec("ALTER TABLE `products` ADD COLUMN `max_per_user` INT DEFAULT 0"); err != nil {
				return fmt.Errorf("添加 products.max_per_user 列失败: %w", err)
			}
			global.APP_LOG.Info("products.max_per_user 列添加完成")
		} else {
			global.APP_LOG.Debug("products.max_per_user 列已存在，跳过")
		}
	} else {
		global.APP_LOG.Debug("products 表不存在，跳过列添加（全新数据库）")
	}

	// === 4. 修复无效的用户等级 (level=999 → level=1) ===
	// level=999 是无效值，通常由旧版本 bug 或数据导入错误导致。
	// 将其修复为普通用户等级 1。
	// 非关键迁移：失败时记录警告但不中断。
	if db.Migrator().HasTable("users") {
		result := db.Exec("UPDATE `users` SET `level` = 1 WHERE `level` = 999")
		if result.Error != nil {
			global.APP_LOG.Warn("修复无效用户等级失败", zap.Error(result.Error))
		} else if result.RowsAffected > 0 {
			global.APP_LOG.Info("已修复无效用户等级",
				zap.Int64("affected_rows", result.RowsAffected))
		} else {
			global.APP_LOG.Debug("无需修复无效用户等级（没有 level=999 的用户）")
		}
	} else {
		global.APP_LOG.Debug("users 表不存在，跳过用户等级修复（全新数据库）")
	}

	// === 5. 为 products 表添加 default_provider_id 和 default_image_id 列 ===
	if db.Migrator().HasTable("products") {
		columns, err := ds.getTableColumns(db, "products")
		if err != nil {
			return fmt.Errorf("获取 products 表列信息失败: %w", err)
		}

		// 添加 default_provider_id 列（默认节点ID）
		if !columns["default_provider_id"] {
			global.APP_LOG.Info("正在为 products 表添加 default_provider_id 列")
			if _, err := sqlDB.Exec("ALTER TABLE `products` ADD COLUMN `default_provider_id` INT DEFAULT 0"); err != nil {
				return fmt.Errorf("添加 products.default_provider_id 列失败: %w", err)
			}
			global.APP_LOG.Info("products.default_provider_id 列添加完成")
		} else {
			global.APP_LOG.Debug("products.default_provider_id 列已存在，跳过")
		}

		// 添加 default_image_id 列（默认镜像ID）
		if !columns["default_image_id"] {
			global.APP_LOG.Info("正在为 products 表添加 default_image_id 列")
			if _, err := sqlDB.Exec("ALTER TABLE `products` ADD COLUMN `default_image_id` INT DEFAULT 0"); err != nil {
				return fmt.Errorf("添加 products.default_image_id 列失败: %w", err)
			}
			global.APP_LOG.Info("products.default_image_id 列添加完成")
		} else {
			global.APP_LOG.Debug("products.default_image_id 列已存在，跳过")
		}
	}

	// === 6. 为 yipay_configs 表添加 enabled_pay_types 列 ===
	if db.Migrator().HasTable("yipay_configs") {
		columns, err := ds.getTableColumns(db, "yipay_configs")
		if err != nil {
			global.APP_LOG.Warn("获取 yipay_configs 表列信息失败，跳过 enabled_pay_types 列添加", zap.Error(err))
		} else if !columns["enabled_pay_types"] {
			global.APP_LOG.Info("正在为 yipay_configs 表添加 enabled_pay_types 列")
			if _, err := sqlDB.Exec("ALTER TABLE `yipay_configs` ADD COLUMN `enabled_pay_types` VARCHAR(128) DEFAULT 'alipay,wxpay,qqpay'"); err != nil {
				global.APP_LOG.Warn("添加 yipay_configs.enabled_pay_types 列失败", zap.Error(err))
			} else {
				global.APP_LOG.Info("yipay_configs.enabled_pay_types 列添加完成")
			}
		} else {
			global.APP_LOG.Debug("yipay_configs.enabled_pay_types 列已存在，跳过")
		}
	}

	return nil
}
