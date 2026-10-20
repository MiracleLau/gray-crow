package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/model"
)

func newDiagnoseFixture(t *testing.T) (*RuleService, *model.Rule, *model.User) {
	t.Helper()
	dsn := fmt.Sprintf("file:diag%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.Rule{}, &model.RuleViewer{}, &model.RuleChannel{}, &model.Alert{}); err != nil {
		t.Fatal(err)
	}
	rule := &model.Rule{Name: "r", Keywords: []string{"acme.com"}, IntervalSec: 3600, Enabled: true, DryRun: true, NextScanAt: time.Now().UTC()}
	if err := db.Create(rule).Error; err != nil {
		t.Fatal(err)
	}
	admin := &model.User{Username: "admin", PasswordHash: "x", Role: model.RoleAdmin, Status: 1}
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}
	return NewRuleService(db, 60, time.UTC), rule, admin
}

func addAlerts(t *testing.T, svc *RuleService, ruleID uint, keyword, detector, status string, n int) {
	t.Helper()
	rows := make([]model.Alert, 0, n)
	for i := 0; i < n; i++ {
		rows = append(rows, model.Alert{
			RuleID:         ruleID,
			RepoFullName:   "acme/repo",
			MatchedKeyword: keyword,
			DetectorID:     detector,
			Status:         status,
			Fingerprint:    fmt.Sprintf("%s-%s-%d-%d", keyword, detector, i, time.Now().UnixNano()),
			FirstSeenAt:    time.Now().UTC(),
			LastSeenAt:     time.Now().UTC(),
		})
	}
	if err := svc.db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
}

func hasSuggestion(d *RuleDiagnosis, kind, keyword string) bool {
	for _, s := range d.Suggestions {
		if s.Kind == kind && (keyword == "" || s.Keyword == keyword) {
			return true
		}
	}
	return false
}

func TestDiagnoseEmpty(t *testing.T) {
	svc, rule, admin := newDiagnoseFixture(t)
	out, err := svc.Diagnose(rule.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 0 || !hasSuggestion(out, "no_data", "") {
		t.Fatalf("空数据应给 no_data: %+v", out.Suggestions)
	}
}

func TestDiagnoseSuggestsDropKeywordAndKeepDryRun(t *testing.T) {
	svc, rule, admin := newDiagnoseFixture(t)
	// 噪声关键词：25 条都只命中关键词、无密钥形状，且判为误报
	addAlerts(t, svc, rule.ID, "acme.com", "", model.AlertFalsePositive, 25)
	// 有效关键词：15 条二次判定命中，已被处置
	addAlerts(t, svc, rule.ID, "acme-internal-key", "generic-credential", model.AlertResolved, 15)

	out, err := svc.Diagnose(rule.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	if out.Total != 40 || out.Detected != 15 || out.ContextOnly != 25 {
		t.Fatalf("统计不符: total=%d detected=%d context_only=%d", out.Total, out.Detected, out.ContextOnly)
	}
	if out.FalsePositiveRate != 0.63 {
		t.Fatalf("误报率应为 25/40=0.63，实际 %v", out.FalsePositiveRate)
	}
	if !hasSuggestion(out, "drop_keyword", "acme.com") {
		t.Fatalf("应建议移除噪声关键词: %+v", out.Suggestions)
	}
	if hasSuggestion(out, "drop_keyword", "acme-internal-key") {
		t.Fatalf("有效关键词不应被建议移除: %+v", out.Suggestions)
	}
	if !hasSuggestion(out, "keep_dry_run", "") {
		t.Fatalf("高误报率应建议保持干跑: %+v", out.Suggestions)
	}
	if len(out.ByKeyword) != 2 || out.ByKeyword[0].Keyword != "acme.com" {
		t.Fatalf("关键词应按命中数降序: %+v", out.ByKeyword)
	}
}

func TestDiagnoseReadyToPublish(t *testing.T) {
	svc, rule, admin := newDiagnoseFixture(t)
	addAlerts(t, svc, rule.ID, "acme.com", "generic-credential", model.AlertResolved, 12)
	out, err := svc.Diagnose(rule.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	if !hasSuggestion(out, "ready", "") {
		t.Fatalf("低误报高置信应建议关闭干跑: %+v", out.Suggestions)
	}
}
