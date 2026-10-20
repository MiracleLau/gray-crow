package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/config"
	"graycrow/internal/model"
)

func Open(cfg *config.Config) (*gorm.DB, error) {
	level := logger.Warn
	if cfg.LogLevel == "debug" {
		level = logger.Info
	}

	var dial gorm.Dialector
	switch cfg.DBDriver {
	case "sqlite":
		dsn, err := prepareSQLite(cfg.DBDSN)
		if err != nil {
			return nil, err
		}
		dial = sqlite.Open(dsn)
	case "mysql":
		dial = mysql.Open(cfg.DBDSN)
	case "postgres":
		dial = postgres.Open(cfg.DBDSN)
	default:
		return nil, fmt.Errorf("不支持的 DB_DRIVER: %s", cfg.DBDriver)
	}

	db, err := gorm.Open(dial, &gorm.Config{
		Logger:         logger.Default.LogMode(level),
		TranslateError: true,
		NowFunc:        func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if cfg.DBDriver == "sqlite" {
		sqlDB.SetMaxOpenConns(1) // ponytail: sqlite 单写者，连接池 >1 只会换锁竞争为 SQLITE_BUSY
	} else {
		sqlDB.SetMaxOpenConns(20)
	}
	return db, nil
}

func prepareSQLite(dsn string) (string, error) {
	if dir := filepath.Dir(dsn); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return "", fmt.Errorf("创建数据目录失败: %w", err)
		}
	}
	if strings.Contains(dsn, "?") {
		return dsn, nil
	}
	return dsn + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(0)", nil
}

func Migrate(db *gorm.DB) error {
	// 只加列不删列，保证旧二进制可回滚读新库
	if err := db.AutoMigrate(
		&model.User{},
		&model.Session{},
		&model.RecoveryCode{},
		&model.APIKey{},
		&model.Rule{},
		&model.RuleChannel{},
		&model.RuleViewer{},
		&model.Allowlist{},
		&model.Channel{},
		&model.NotifyTemplate{},
		&model.Setting{},
		&model.AuditLog{},
		&model.Alert{},
		&model.AlertEvent{},
		&model.ScanRun{},
		&model.NotifyRecord{},
	); err != nil {
		return err
	}
	// 例外：告警的级别/来源/重复次数已下线，显式删列，避免旧库继续留存无用数据
	return dropColumns(db, &model.Alert{}, "severity", "dry_run", "seen_count")
}

func dropColumns(db *gorm.DB, model any, columns ...string) error {
	m := db.Migrator()
	for _, column := range columns {
		if !m.HasColumn(model, column) {
			continue
		}
		if err := m.DropColumn(model, column); err != nil {
			return err
		}
	}
	return nil
}

func SeedAdmin(db *gorm.DB, username, password string) (bool, error) {
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return false, err
	}
	created := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Nickname:     "系统管理员",
		Role:         model.RoleAdmin,
		Status:       1,
	}
	if err := db.Create(created).Error; err != nil {
		return false, err
	}
	return true, nil
}

// SeedNotifyTemplate 建库时写入可编辑的默认汇总模板；已有默认模板时不动。
func SeedNotifyTemplate(db *gorm.DB) (bool, error) {
	var count int64
	if err := db.Model(&model.NotifyTemplate{}).Where("is_default = ?", true).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	row := &model.NotifyTemplate{
		Name:      "默认模板",
		IsDefault: true,
		Title:     "【灰鸦告警汇总】{{title}}：{{count}} 条新问题",
		Body:      "【灰鸦告警汇总】{{title}}：本次扫描发现 {{count}} 条新问题\n仓库：{{repos}}\n{{items}}",
	}
	if err := db.Create(row).Error; err != nil {
		return false, err
	}
	return true, nil
}
