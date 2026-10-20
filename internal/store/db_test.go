package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/model"
)

// legacyAlert 是下线 severity/dry_run/seen_count 之前的告警表结构。
type legacyAlert struct {
	ID           uint   `gorm:"primaryKey"`
	RuleID       uint   `gorm:"index"`
	RepoFullName string `gorm:"size:191;index"`
	Severity     string `gorm:"size:16;index"`
	Status       string `gorm:"size:16;not null;default:pending;index"`
	DryRun       bool   `gorm:"not null;default:false"`
	Fingerprint  string `gorm:"size:64;uniqueIndex"`
	SeenCount    int    `gorm:"not null;default:1"`
}

func (legacyAlert) TableName() string { return "alerts" }

// 旧库带着已下线的告警列，Migrate 应把它们删掉且不动其余数据。
func TestMigrateDropsRetiredAlertColumns(t *testing.T) {
	dsn := fmt.Sprintf("file:migrate%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)

	if err := db.AutoMigrate(&legacyAlert{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&legacyAlert{
		RuleID: 1, RepoFullName: "acme/backend", Severity: "high",
		Status: model.AlertPending, DryRun: true, Fingerprint: "fp-1", SeenCount: 3,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	for _, column := range []string{"severity", "dry_run", "seen_count"} {
		if db.Migrator().HasColumn(&model.Alert{}, column) {
			t.Fatalf("已下线列 %s 应被删除", column)
		}
	}
	var repo string
	if err := db.Raw("SELECT repo_full_name FROM alerts WHERE fingerprint = ?", "fp-1").Scan(&repo).Error; err != nil {
		t.Fatal(err)
	}
	if repo != "acme/backend" {
		t.Fatalf("迁移不应丢数据，实际 %q", repo)
	}
}
