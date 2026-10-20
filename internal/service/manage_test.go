package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"graycrow/internal/crypto"
	"graycrow/internal/model"
)

var manageDBCounter int64

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:manage%d?mode=memory&cache=shared", atomic.AddInt64(&manageDBCounter, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("打开内存库失败: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.User{}, &model.Rule{}, &model.RuleViewer{}, &model.RuleChannel{},
		&model.Channel{}, &model.NotifyTemplate{}, &model.Allowlist{}, &model.Setting{}, &model.Session{},
		&model.APIKey{}, &model.Alert{}, &model.AlertEvent{}, &model.NotifyRecord{}, &model.RuleChannel{},
		&model.ScanRun{}, &model.AuditLog{}); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	if err := crypto.Init("unit-test-master-key-0123456789abcdef"); err != nil {
		t.Fatalf("初始化加密模块失败: %v", err)
	}
	return db
}

func adminActor() *model.User {
	return &model.User{ID: 1, Username: "admin", Nickname: "管理员", Role: model.RoleAdmin, Status: 1}
}

func TestRuleValidationAndJSONRoundTrip(t *testing.T) {
	db := newTestDB(t)
	rules := NewRuleService(db, 60, time.UTC)
	actor := adminActor()

	created, err := rules.Create(actor, RuleInput{
		Name: "  acme 巡检 ", Keywords: []string{"acme.com", " acme.com ", "", "@acme.com"},
		IntervalSec: 60, Severity: "weird", Enabled: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}
	if len(created.Keywords) != 2 {
		t.Fatalf("关键词应去重去空，实际 %v", created.Keywords)
	}
	if created.Severity != "high" {
		t.Fatalf("非法级别应回退 high，实际 %s", created.Severity)
	}
	if created.Name != "acme 巡检" {
		t.Fatalf("名称应去首尾空格，实际 %q", created.Name)
	}

	// 回归：map 更新会绕过 serializer:json，导致 keywords 以括号形式写进 SQL
	updated, err := rules.Update(created.ID, RuleInput{
		Name: "改名", Keywords: []string{"b.com", "c.com"}, IntervalSec: 120, Severity: "low", Enabled: false,
	})
	if err != nil {
		t.Fatalf("更新规则失败: %v", err)
	}
	if len(updated.Keywords) != 2 || updated.Keywords[1] != "c.com" {
		t.Fatalf("更新后关键词异常: %v", updated.Keywords)
	}
	if updated.IntervalSec != 120 || updated.Enabled {
		t.Fatalf("更新未生效: %+v", updated)
	}

	if _, err := rules.Create(actor, RuleInput{Name: "x", Keywords: []string{"a.com"}, IntervalSec: 10}); !errors.Is(err, ErrIntervalTooLow) {
		t.Fatalf("低于下限应报错，实际 %v", err)
	}
	if _, err := rules.Create(actor, RuleInput{Name: "x", Keywords: []string{"  "}, IntervalSec: 60}); !errors.Is(err, ErrKeywords) {
		t.Fatalf("空关键词应报错，实际 %v", err)
	}
	if _, err := rules.Create(actor, RuleInput{Name: "  ", Keywords: []string{"a.com"}, IntervalSec: 60}); !errors.Is(err, ErrRuleName) {
		t.Fatalf("空名称应报错，实际 %v", err)
	}

	// 回归：模型字段带 default 标签时 GORM 会省略零值，显式传 false 必须真的落库为 false
	off, err := rules.Create(actor, RuleInput{Name: "默认停用", Keywords: []string{"off.com"}, IntervalSec: 60})
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled || off.DryRun {
		t.Fatalf("未指定时应保持零值，实际 enabled=%v dry_run=%v", off.Enabled, off.DryRun)
	}
	reload, err := rules.Get(off.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if reload.Enabled || reload.DryRun {
		t.Fatalf("读取回的开关状态被 DB 默认值污染: %+v", reload)
	}
}

func TestRuleCronSchedule(t *testing.T) {
	db := newTestDB(t)
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	rules := NewRuleService(db, 60, loc)
	actor := adminActor()
	no := false

	created, err := rules.Create(actor, RuleInput{
		Name: "每日巡检", Keywords: []string{"acme.com"},
		ScheduleType: "cron", CronExpr: "0 9 * * *", RunNow: &no, Enabled: true, DryRun: true,
	})
	if err != nil {
		t.Fatalf("创建 cron 规则失败: %v", err)
	}
	local := created.NextScanAt.In(loc)
	if local.Hour() != 9 || local.Minute() != 0 || !created.NextScanAt.After(time.Now()) {
		t.Fatalf("下次扫描应是上海时间 9:00，实际 %v", local)
	}

	now, err := rules.Create(actor, RuleInput{
		Name: "立即跑", Keywords: []string{"b.com"}, ScheduleType: "cron", CronExpr: "0 9 * * *", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(now.NextScanAt) > time.Minute {
		t.Fatalf("未传 run_now 时应立即扫描，实际 %v", now.NextScanAt)
	}

	if _, err := rules.Create(actor, RuleInput{
		Name: "坏表达式", Keywords: []string{"c.com"}, ScheduleType: "cron", CronExpr: "* * * 中", IntervalSec: 60,
	}); !errors.Is(err, ErrScheduleInvalid) {
		t.Fatalf("非法 cron 应被拒绝，实际 %v", err)
	}
	if _, err := rules.Create(actor, RuleInput{
		Name: "永不触发", Keywords: []string{"c.com"}, ScheduleType: "cron", CronExpr: "0 0 31 2 *",
	}); !errors.Is(err, ErrScheduleInvalid) {
		t.Fatalf("永不触发的 cron 应被拒绝，实际 %v", err)
	}

	updated, err := rules.Update(created.ID, RuleInput{
		Name: "改成间隔", Keywords: []string{"acme.com"}, ScheduleType: "interval", IntervalSec: 3600, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ScheduleType != "interval" || updated.CronExpr != "" {
		t.Fatalf("调度方式未切换: %+v", updated.Rule)
	}
	if until := time.Until(updated.NextScanAt); until <= 0 || until > time.Hour+time.Minute {
		t.Fatalf("切到间隔后应重算下一跑，实际 %v", until)
	}
}

func TestRuleScopeAndBindings(t *testing.T) {
	db := newTestDB(t)
	rules := NewRuleService(db, 60, time.UTC)
	admin := adminActor()
	viewer := &model.User{ID: 2, Username: "alice", Role: model.RoleViewer, Status: 1}

	rule, err := rules.Create(admin, RuleInput{Name: "r", Keywords: []string{"a.com"}, IntervalSec: 60, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Get(rule.ID, viewer); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("未授权 viewer 应越权失败，实际 %v", err)
	}

	if err := rules.SetViewers(rule.ID, []uint{2, 2, 0}); err != nil {
		t.Fatal(err)
	}
	if err := rules.SetChannels(rule.ID, []uint{7}); err != nil {
		t.Fatal(err)
	}
	got, err := rules.Get(rule.ID, viewer)
	if err != nil {
		t.Fatalf("授权后应可读: %v", err)
	}
	if len(got.ViewerIDs) != 1 || len(got.ChannelIDs) != 1 {
		t.Fatalf("绑定异常: viewers=%v channels=%v", got.ViewerIDs, got.ChannelIDs)
	}

	visible, err := rules.VisibleRuleIDs(viewer)
	if err != nil || len(visible) != 1 || visible[0] != rule.ID {
		t.Fatalf("可见规则异常: %v err=%v", visible, err)
	}

	if err := rules.SetViewers(rule.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rules.Get(rule.ID, viewer); !errors.Is(err, ErrOutOfScope) {
		t.Fatalf("解绑后应越权失败，实际 %v", err)
	}
}

func TestChannelConfigRoundTrip(t *testing.T) {
	db := newTestDB(t)
	channels := NewChannelService(db)

	secret := `{"X-Token":"abc"}`
	row, err := channels.Create(adminActor(), ChannelInput{
		Name: "网关", Type: "webhook", Enabled: true,
		Config: map[string]any{"url": "https://example.com/hook", "method": "POST"},
		Secret: &secret,
	})
	if err != nil {
		t.Fatalf("创建渠道失败: %v", err)
	}
	updated, err := channels.Update(row.ID, ChannelInput{
		Name: "网关2", Type: "feishu", Enabled: false,
		Config: map[string]any{"webhook": "https://open.feishu.cn/x", "at": []any{"1"}},
	})
	if err != nil {
		t.Fatalf("更新渠道失败: %v", err)
	}
	if updated.Config["url_hint"] != "https://open.feishu.cn/****" || updated.Name != "网关2" || updated.Enabled {
		t.Fatalf("更新未生效: %+v", updated.Config)
	}
	if updated.SecretEnc == "" {
		t.Fatal("地址应保留在密文中")
	}
	if _, err := channels.Create(adminActor(), ChannelInput{Name: "x", Type: "sms"}); !errors.Is(err, ErrChannelType) {
		t.Fatalf("非法类型应报错，实际 %v", err)
	}
	off, err := channels.Create(adminActor(), ChannelInput{Name: "停用渠道", Type: "webhook", Enabled: false, Config: map[string]any{"url": "https://example.com/off"}})
	if err != nil {
		t.Fatal(err)
	}
	if off.Enabled {
		t.Fatal("显式停用的渠道不应被 DB 默认值打开")
	}
}

func TestAllowlistAndSettings(t *testing.T) {
	db := newTestDB(t)
	allows := NewAllowService(db)

	ruleID := uint(3)
	if _, err := allows.Create(adminActor(), "global", &ruleID, "badformat", "", nil); !errors.Is(err, ErrRepoFormat) {
		t.Fatalf("非法仓库应报错，实际 %v", err)
	}
	if _, err := allows.Create(adminActor(), "rule", nil, "a/b", "", nil); !errors.Is(err, ErrScopeInvalid) {
		t.Fatalf("rule 级缺 rule_id 应报错，实际 %v", err)
	}
	row, err := allows.Create(adminActor(), "rule", &ruleID, "a/b", "示例", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := allows.Delete(row.ID); err != nil {
		t.Fatalf("删除失败: %v", err)
	}
	if err := allows.Delete(row.ID); !errors.Is(err, ErrAllowNotFound) {
		t.Fatalf("重复删除应报不存在，实际 %v", err)
	}

	settings := NewSettingsService(db)
	values, err := settings.Get()
	if err != nil {
		t.Fatal(err)
	}
	if values["default_interval_sec"] != 3600 {
		t.Fatalf("默认值异常: %v", values)
	}
	if err := settings.Update(map[string]any{"default_interval_sec": float64(5)}); !errors.Is(err, ErrSettingInvalid) {
		t.Fatalf("间隔过小应报错，实际 %v", err)
	}
	if err := settings.Update(map[string]any{"default_interval_sec": float64(120), "unknown_key": "x"}); err != nil {
		t.Fatalf("更新失败: %v", err)
	}
	values, _ = settings.Get()
	if values["default_interval_sec"] != 120 {
		t.Fatalf("更新未生效: %v", values)
	}
}

func TestUserGuards(t *testing.T) {
	db := newTestDB(t)
	users := NewUserService(db)
	admin := adminActor()
	if err := db.Create(admin).Error; err != nil {
		t.Fatal(err)
	}

	if _, err := users.Create(UserInput{Username: "alice", Role: "viewer", Password: "123"}); !errors.Is(err, ErrPasswordShort) {
		t.Fatalf("短密码应报错，实际 %v", err)
	}
	viewer, err := users.Create(UserInput{Username: "alice", Nickname: "Alice", Role: "viewer", Password: "viewer123"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(UserInput{Username: "alice", Role: "viewer", Password: "viewer123"}); !errors.Is(err, ErrUserDuplicate) {
		t.Fatalf("重名应报错，实际 %v", err)
	}
	if err := users.Delete(admin, admin.ID); !errors.Is(err, ErrSelfOperation) {
		t.Fatalf("删除自己应报错，实际 %v", err)
	}
	if _, err := users.Update(admin.ID, UserInput{Role: "viewer"}); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("降级唯一管理员应报错，实际 %v", err)
	}
	if err := users.ResetPassword(viewer.ID, "newpass123"); err != nil {
		t.Fatalf("重置密码失败: %v", err)
	}
	if err := users.Delete(admin, viewer.ID); err != nil {
		t.Fatalf("删除普通用户失败: %v", err)
	}
}

func TestRetentionKeepsManualHistory(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	old := now.AddDate(0, 0, -400)

	if err := db.Create(&model.ScanRun{RuleID: 1, Status: model.RunSuccess, StartedAt: old}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ScanRun{RuleID: 1, Status: model.RunSuccess, StartedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.AuditLog{Action: "old", CreatedAt: old}).Error; err != nil {
		t.Fatal(err)
	}
	alert := &model.Alert{RuleID: 1, RepoFullName: "a/b", Status: model.AlertPending, Fingerprint: "retention-fp", LastSeenAt: old}
	if err := db.Create(alert).Error; err != nil {
		t.Fatal(err)
	}
	rec := &model.NotifyRecord{RuleID: 1, ChannelID: 1, Status: "success", SentAt: old}
	if err := db.Create(rec).Error; err != nil {
		t.Fatal(err)
	}

	counts, err := NewRetention(db, NewAudit(db)).RunOnce()
	if err != nil {
		t.Fatalf("清理失败: %v", err)
	}
	if counts["scan_runs"] != 1 || counts["audit_logs"] != 1 {
		t.Fatalf("清理条数异常: %v", counts)
	}
	var n int64
	db.Model(&model.ScanRun{}).Count(&n)
	if n != 1 {
		t.Fatalf("扫描日志应只留新记录，实际 %d", n)
	}
	db.Model(&model.Alert{}).Count(&n)
	if n != 1 {
		t.Fatal("告警不应被自动清理")
	}
	db.Model(&model.NotifyRecord{}).Count(&n)
	if n != 1 {
		t.Fatal("通知记录不应被自动清理")
	}
}

// 回归:禁用规则的「立即扫描」曾静默写入 next_scan_at,启用后被调度器补跑一次。
func TestRuleScanTriggerRespectsEnabled(t *testing.T) {
	db := newTestDB(t)
	rules := NewRuleService(db, 60, time.UTC)
	actor := adminActor()

	created, err := rules.Create(actor, RuleInput{Name: "停用中", Keywords: []string{"a.com"}, IntervalSec: 3600})
	if err != nil {
		t.Fatalf("创建规则失败: %v", err)
	}
	if created.Enabled {
		t.Fatal("未指定 enabled 时应默认停用")
	}

	if err := rules.TriggerScan(created.ID); !errors.Is(err, ErrRuleDisabled) {
		t.Fatalf("禁用规则触发扫描应报错,实际 %v", err)
	}
	after, err := rules.Get(created.ID, actor)
	if err != nil {
		t.Fatal(err)
	}
	if !after.NextScanAt.Equal(created.NextScanAt) {
		t.Fatalf("禁用规则不应被写入 next_scan_at:%v → %v", created.NextScanAt, after.NextScanAt)
	}

	enabled, err := rules.Update(created.ID, RuleInput{Name: "停用中", Keywords: []string{"a.com"}, IntervalSec: 3600, Enabled: true})
	if err != nil {
		t.Fatalf("启用规则失败: %v", err)
	}
	if !enabled.NextScanAt.After(time.Now().UTC()) {
		t.Fatalf("启用时应重算下一跑,实际 %v", enabled.NextScanAt)
	}
}
