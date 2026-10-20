package service

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"

	"graycrow/internal/model"
)

const (
	scanRunRetentionDays = 30
	auditRetentionDays   = 180
)

// Retention 自动清理扫描日志与审计日志；告警和通知记录不自动删，由管理员手动清理。
type Retention struct {
	db    *gorm.DB
	audit *Audit
}

func NewRetention(db *gorm.DB, audit *Audit) *Retention {
	return &Retention{db: db, audit: audit}
}

// Start 每天清理一次；首轮延迟 1 分钟，避开启动高峰。
func (s *Retention) Start(ctx context.Context) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if counts, err := s.RunOnce(); err != nil {
				log.Printf("数据清理失败: %v", err)
			} else if counts["scan_runs"]+counts["audit_logs"] > 0 {
				log.Printf("数据清理完成: 扫描日志 %d 条、审计日志 %d 条", counts["scan_runs"], counts["audit_logs"])
			}
			timer.Reset(24 * time.Hour)
		}
	}
}

func (s *Retention) RunOnce() (map[string]int64, error) {
	counts := map[string]int64{"scan_runs": 0, "audit_logs": 0}
	now := time.Now().UTC()

	runs := s.db.Where("started_at < ?", now.AddDate(0, 0, -scanRunRetentionDays)).Delete(&model.ScanRun{})
	if runs.Error != nil {
		return counts, runs.Error
	}
	counts["scan_runs"] = runs.RowsAffected

	logs := s.db.Where("created_at < ?", now.AddDate(0, 0, -auditRetentionDays)).Delete(&model.AuditLog{})
	if logs.Error != nil {
		return counts, logs.Error
	}
	counts["audit_logs"] = logs.RowsAffected

	s.audit.Record(nil, "retention.run", "system", "扫描日志与审计日志清理完成", "")
	return counts, nil
}
